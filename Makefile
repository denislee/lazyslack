.PHONY: all build run clean

# Default target
all: build

# Build the lazyslack binary
build:
	go build -o lazyslack ./cmd/lazyslack

# Build and run the application
run: build
	./lazyslack

# Clean the build output
clean:
	rm -f lazyslack
