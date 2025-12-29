package queue

import (
	"context"
	"fmt"
	"time"

	"borg/mothership/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Queue manages job queue and task distribution
type Queue struct {
	db *gorm.DB
}

// NewQueue creates a new queue instance
func NewQueue(db *gorm.DB) *Queue {
	return &Queue{db: db}
}

// EnqueueJob creates a new job and initial task
func (q *Queue) EnqueueJob(job *models.Job) error {
	job.ID = uuid.New().String()
	job.Status = "pending"
	job.CreatedAt = time.Now()
	job.UpdatedAt = time.Now()

	if err := q.db.Create(job).Error; err != nil {
		return fmt.Errorf("failed to create job: %w", err)
	}

	// Create initial task
	task := &models.Task{
		ID:         uuid.New().String(),
		JobID:      job.ID,
		Status:     "pending",
		RetryCount: 0,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	if err := q.db.Create(task).Error; err != nil {
		return fmt.Errorf("failed to create task: %w", err)
	}

	return nil
}

// GetNextTask retrieves the next pending task for a runner
func (q *Queue) GetNextTask(runnerID string, runnerCapabilities map[string]interface{}) (*models.Task, error) {
	var task models.Task

	// Find pending task from a pending or running job, ordered by creation time
	// Only assign tasks from jobs that are pending or running (not paused, cancelled, etc.)
	// Use a subquery to filter by job status
	query := q.db.
		Where("status = ?", "pending").
		Where("job_id IN (SELECT id FROM jobs WHERE status IN (?))", []string{"pending", "running"}).
		Order("created_at ASC").
		First(&task)

	if query.Error != nil {
		if query.Error == gorm.ErrRecordNotFound {
			return nil, nil // No pending tasks
		}
		return nil, fmt.Errorf("failed to get next task: %w", query.Error)
	}

	// Preload the job relation
	if err := q.db.Preload("Job").First(&task, "id = ?", task.ID).Error; err != nil {
		return nil, fmt.Errorf("failed to load task job: %w", err)
	}

	// Lock the task by updating its status
	now := time.Now()
	task.RunnerID = runnerID
	task.Status = "running"
	task.StartedAt = &now
	task.UpdatedAt = now

	if err := q.db.Save(&task).Error; err != nil {
		return nil, fmt.Errorf("failed to lock task: %w", err)
	}

	// Update job status
	q.db.Model(&models.Job{}).Where("id = ?", task.JobID).Update("status", "running")

	return &task, nil
}

// UpdateTaskStatus updates task status and handles retries
func (q *Queue) UpdateTaskStatus(taskID string, status string, exitCode *int32, errorMessage string) error {
	var task models.Task
	if err := q.db.First(&task, "id = ?", taskID).Error; err != nil {
		return fmt.Errorf("task not found: %w", err)
	}

	now := time.Now()
	task.Status = status
	task.ExitCode = exitCode
	task.ErrorMessage = errorMessage
	task.UpdatedAt = now

	if status == "completed" || status == "failed" || status == "cancelled" {
		task.CompletedAt = &now
	}

	if err := q.db.Save(&task).Error; err != nil {
		return fmt.Errorf("failed to update task: %w", err)
	}

	// Handle retries
	if status == "failed" {
		var job models.Job
		if err := q.db.First(&job, "id = ?", task.JobID).Error; err == nil {
			if task.RetryCount < job.MaxRetries {
				// Create new task for retry
				newTask := &models.Task{
					ID:         uuid.New().String(),
					JobID:      task.JobID,
					Status:     "pending",
					RetryCount: task.RetryCount + 1,
					CreatedAt:  time.Now(),
					UpdatedAt:  time.Now(),
				}
				if err := q.db.Create(newTask).Error; err != nil {
					return fmt.Errorf("failed to create retry task: %w", err)
				}
			} else {
				// Max retries reached, mark job as failed
				q.db.Model(&job).Update("status", "failed")
			}
		}
	} else if status == "completed" {
		// Check if all tasks for this job are completed
		var count int64
		q.db.Model(&models.Task{}).Where("job_id = ? AND status NOT IN (?)", task.JobID, []string{"completed", "failed", "cancelled"}).Count(&count)
		if count == 0 {
			q.db.Model(&models.Job{}).Where("id = ?", task.JobID).Update("status", "completed")
		}
	}

	return nil
}

// PauseJob pauses a job and its running tasks
func (q *Queue) PauseJob(jobID string) error {
	// Update job status
	if err := q.db.Model(&models.Job{}).Where("id = ?", jobID).Update("status", "paused").Error; err != nil {
		return fmt.Errorf("failed to pause job: %w", err)
	}

	// Update running tasks to paused
	if err := q.db.Model(&models.Task{}).
		Where("job_id = ? AND status = ?", jobID, "running").
		Update("status", "paused").Error; err != nil {
		return fmt.Errorf("failed to pause tasks: %w", err)
	}

	return nil
}

// ResumeJob resumes a paused job
func (q *Queue) ResumeJob(jobID string) error {
	// Update job status
	if err := q.db.Model(&models.Job{}).Where("id = ? AND status = ?", jobID, "paused").Update("status", "pending").Error; err != nil {
		return fmt.Errorf("failed to resume job: %w", err)
	}

	// Create new tasks for paused tasks
	var pausedTasks []models.Task
	if err := q.db.Where("job_id = ? AND status = ?", jobID, "paused").Find(&pausedTasks).Error; err != nil {
		return fmt.Errorf("failed to find paused tasks: %w", err)
	}

	for _, task := range pausedTasks {
		// Create new pending task
		newTask := &models.Task{
			ID:         uuid.New().String(),
			JobID:      task.JobID,
			Status:     "pending",
			RetryCount: task.RetryCount,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}
		if err := q.db.Create(newTask).Error; err != nil {
			return fmt.Errorf("failed to create resumed task: %w", err)
		}

		// Mark old task as cancelled
		q.db.Model(&task).Update("status", "cancelled")
	}

	return nil
}

// CancelJob cancels a job and all its tasks
func (q *Queue) CancelJob(jobID string) error {
	// Update job status
	if err := q.db.Model(&models.Job{}).Where("id = ?", jobID).Update("status", "cancelled").Error; err != nil {
		return fmt.Errorf("failed to cancel job: %w", err)
	}

	// Update all non-terminal tasks to cancelled
	now := time.Now()
	if err := q.db.Model(&models.Task{}).
		Where("job_id = ? AND status NOT IN (?)", jobID, []string{"completed", "failed", "cancelled"}).
		Updates(map[string]interface{}{
			"status":       "cancelled",
			"completed_at": now,
			"updated_at":   now,
		}).Error; err != nil {
		return fmt.Errorf("failed to cancel tasks: %w", err)
	}

	return nil
}

// GetJob returns a job by ID
func (q *Queue) GetJob(jobID string) (*models.Job, error) {
	var job models.Job
	if err := q.db.Preload("Tasks").Preload("JobFiles").First(&job, "id = ?", jobID).Error; err != nil {
		return nil, fmt.Errorf("job not found: %w", err)
	}
	return &job, nil
}

// ListJobs returns a list of jobs with pagination
func (q *Queue) ListJobs(limit, offset int, status string) ([]models.Job, int64, error) {
	var jobs []models.Job
	var total int64

	query := q.db.Model(&models.Job{})
	if status != "" {
		query = query.Where("status = ?", status)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count jobs: %w", err)
	}

	if err := query.Order("created_at DESC").Limit(limit).Offset(offset).Find(&jobs).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to list jobs: %w", err)
	}

	return jobs, total, nil
}

// GetStats returns queue statistics
func (q *Queue) GetStats(ctx context.Context) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Count jobs by status
	jobCounts := make(map[string]int64)
	statuses := []string{"pending", "running", "paused", "completed", "failed", "cancelled"}
	for _, status := range statuses {
		var count int64
		q.db.Model(&models.Job{}).Where("status = ?", status).Count(&count)
		jobCounts[status] = count
	}
	stats["jobs"] = jobCounts

	// Count tasks by status
	taskCounts := make(map[string]int64)
	for _, status := range statuses {
		var count int64
		q.db.Model(&models.Task{}).Where("status = ?", status).Count(&count)
		taskCounts[status] = count
	}
	stats["tasks"] = taskCounts

	return stats, nil
}
