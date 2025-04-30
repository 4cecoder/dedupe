.PHONY: build install clean help

# Default target
.DEFAULT_GOAL := help

# Program name
PROGRAM_NAME = dedupe

# Installation directory
INSTALL_DIR = /usr/local/bin

# Go parameters
GOCMD = go
GOBUILD = $(GOCMD) build
GOCLEAN = $(GOCMD) clean
GOTEST = $(GOCMD) test
GOGET = $(GOCMD) get

# Build flags
BUILD_FLAGS = -v

# Get OS information
UNAME_S := $(shell uname -s 2>/dev/null || echo Windows)

# Set executable extension based on OS
ifeq ($(UNAME_S),Windows)
	EXECUTABLE_EXTENSION = .exe
	RM_CMD = del /Q
else
	EXECUTABLE_EXTENSION = 
	RM_CMD = rm -f
endif

# Program name with extension if needed
EXECUTABLE = $(PROGRAM_NAME)$(EXECUTABLE_EXTENSION)

# Help target
help:
	@echo "Available targets:"
	@echo "  build   - Build the $(PROGRAM_NAME) binary"
	@echo "  install - Install $(PROGRAM_NAME) to $(INSTALL_DIR)"
	@echo "  clean   - Remove build artifacts"
	@echo "  help    - Display this help message"

# Build target
build:
	$(GOBUILD) $(BUILD_FLAGS) -o $(EXECUTABLE)

# Install target
install: build
ifeq ($(UNAME_S),Darwin)
	@echo "Installing on macOS..."
	cp $(EXECUTABLE) $(INSTALL_DIR)/$(PROGRAM_NAME)
else ifeq ($(UNAME_S),Linux)
	@echo "Installing on Linux..."
	cp $(EXECUTABLE) $(INSTALL_DIR)/$(PROGRAM_NAME)
else ifeq ($(UNAME_S),Windows)
	@echo "Installing on Windows..."
	@echo "Please copy $(EXECUTABLE) to a directory in your PATH"
	@echo "Example: copy $(EXECUTABLE) C:\Windows\System32\"
else
	@echo "Unsupported OS for automatic installation. Please copy the binary manually to your PATH."
endif

# Clean target
clean:
	$(GOCLEAN)
	$(RM_CMD) $(EXECUTABLE)
