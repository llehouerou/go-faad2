.PHONY: fmt lint test coverage check build install-hooks wasm testdata

# Format all Go files (tools provided by nix devShell)
fmt:
	goimports-reviser -format -recursive .

# Lint
lint:
	golangci-lint run

# Run tests (use PKG=./path/to/package to test specific package)
test:
ifdef PKG
	go test -v $(PKG)
else
	go test ./...
endif

# Run tests with coverage (use PKG=./path/to/package for specific package)
coverage:
ifdef PKG
	go test -cover -coverprofile=coverage.out $(PKG)
else
	go test -cover -coverprofile=coverage.out ./...
endif
	go tool cover -func=coverage.out

# Format, lint, and test
check: fmt lint test

# Build WASM (requires Emscripten)
wasm:
	cd wasm-build && ./build.sh
	cp wasm-build/faad2.wasm .

# Install git hooks
install-hooks:
	cp scripts/pre-commit .git/hooks/pre-commit
	chmod +x .git/hooks/pre-commit

# Generate test audio files (requires ffmpeg)
testdata:
	@mkdir -p testdata
	@echo "Generating test audio files..."
	ffmpeg -y -f lavfi -i "sine=frequency=440:duration=1" \
		-c:a aac -b:a 128k testdata/mono_44100.m4a
	ffmpeg -y -f lavfi -i "sine=frequency=440:duration=1" \
		-ac 2 -ar 48000 -c:a aac -b:a 128k testdata/stereo_48000.m4a
	ffmpeg -y -f lavfi -i "sine=frequency=440:duration=1" \
		-c:a aac -b:a 128k -f adts testdata/test.aac
	ffmpeg -y -f lavfi -i "sine=frequency=440:duration=2" \
		-ac 2 -c:a aac -b:a 128k \
		-metadata title="Test Title" \
		-metadata artist="Test Artist" \
		-metadata album="Test Album" \
		testdata/with_metadata.m4a
	@echo "Test files generated in testdata/"
