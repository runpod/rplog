release:
	mkdir -p bin
	rm -r bin/* || true

	GOOS=linux GOARCH=amd64 go build -o bin/buildmeta_amd64_linux -trimpath -ldflags "-s -w" ./cmd/buildmeta.go
	GOOS=linux GOARCH=arm64 go build -o bin/buildmeta_arm64_linux -trimpath -ldflags "-s -w" ./cmd/buildmeta.go
	GOOS=darwin GOARCH=amd64 go build -o bin/buildmeta_amd64_darwin -trimpath -ldflags "-s -w" ./cmd/buildmeta.go
	GOOS=darwin GOARCH=arm64 go build -o bin/buildmeta_arm64_darwin -trimpath -ldflags "-s -w" ./cmd/buildmeta.go
	GOOS=windows GOARCH=amd64 go build -o bin/buildmeta_amd64_windows -trimpath -ldflags "-s -w" ./cmd/buildmeta.go
	(which upx && upx -7 bin/buildmeta_*) || true
