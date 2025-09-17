#!/bin/bash
set -euo pipefail

# Change to workspace root
cd "$BUILD_WORKSPACE_DIRECTORY"

echo "Running tests with coverage..."
bazel coverage //...

echo "Generating coverage report..."
# Get the correct testlogs directory
TESTLOGS_DIR=$(bazel info bazel-testlogs)

# Find all coverage.dat files and combine them
COVERAGE_FILES=$(find "$TESTLOGS_DIR" -name "coverage.dat")
if [ -z "$COVERAGE_FILES" ]; then
    echo "No coverage files found"
    exit 1
fi

# Combine LCOV coverage files
cat $COVERAGE_FILES > coverage_combined.dat

echo "Coverage files combined: coverage_combined.dat"

# Check if lcov is available for HTML report generation
if command -v lcov &> /dev/null && command -v genhtml &> /dev/null; then
    echo "Generating HTML coverage report with lcov..."
    lcov --summary coverage_combined.dat
    genhtml coverage_combined.dat --output-directory coverage-html
    echo "HTML coverage report generated: coverage-html/index.html"
    
    # Try to open the coverage report (macOS)
    if command -v open &> /dev/null; then
        open coverage.html
    fi
fi