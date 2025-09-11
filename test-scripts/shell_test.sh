#!/bin/bash

# Test script for shell command testing
# This script demonstrates various shell features

echo "Shell test script starting..."

# Test environment variables
echo "Current user: $USER"
echo "Home directory: $HOME"
echo "Working directory: $(pwd)"

# Test command success/failure
echo "Testing successful command:"
ls -la /tmp > /dev/null && echo "ls command succeeded"

echo "Testing command that might fail:"
ls /nonexistent_directory 2>/dev/null || echo "ls command failed as expected"

# Test with different exit codes
echo "Exiting with status 0 (success)"
exit 0
