release:
	GOOS=linux GOARCH=amd64 go build -o buildmeta_amd64_linux -trimpath -ldflags "-s -w" ./go/cmd/buildmeta.go
	GOOS=linux GOARCH=arm64 go build -o buildmeta_arm64_linux -trimpath -ldflags "-s -w" ./go/cmd/buildmeta.go
	GOOS=darwin GOARCH=amd64 go build -o buildmeta_amd64_darwin -trimpath -ldflags "-s -w" ./go/cmd/buildmeta.go
	GOOS=windows GOARCH=amd64 go build -o buildmeta_amd64_windows -trimpath -ldflags "-s -w" ./go/cmd/buildmeta.go
	zstd -19 buildmeta_amd64_linux
	zstd -19 buildmeta_arm64_linux
	zstd -19 buildmeta_amd64_darwin
	zstd -19 buildmeta_amd64_windows