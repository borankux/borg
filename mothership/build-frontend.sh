#!/bin/bash
# Build Frontend for Production

echo "Building Frontend for Production..."
echo ""

cd web || exit 1

# Check if node_modules exists
if [ ! -d "node_modules" ]; then
    echo "Installing dependencies..."
    npm install
    if [ $? -ne 0 ]; then
        echo "ERROR: Failed to install dependencies!"
        exit 1
    fi
fi

# Build frontend
echo "Building frontend..."
npm run build

if [ $? -ne 0 ]; then
    echo "ERROR: Frontend build failed!"
    exit 1
fi

echo ""
echo "✓ Frontend built successfully!"
echo "Build output: web/dist/"
echo ""
echo "You can now run the backend server and it will serve the frontend."

