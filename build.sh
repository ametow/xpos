GOOS=darwin     GOARCH=amd64    go build -ldflags '-s' -o bin/xpos-darwin-amd64       agent/main.go
GOOS=darwin     GOARCH=arm64    go build -ldflags '-s' -o bin/xpos-darwin-arm64       agent/main.go
GOOS=linux      GOARCH=386      go build -ldflags '-s' -o bin/xpos-linux-386          agent/main.go
GOOS=linux      GOARCH=amd64    go build -ldflags '-s' -o bin/xpos-linux-amd64        agent/main.go
GOOS=linux      GOARCH=arm      go build -ldflags '-s' -o bin/xpos-linux-arm          agent/main.go
GOOS=linux      GOARCH=arm64    go build -ldflags '-s' -o bin/xpos-linux-arm64        agent/main.go
GOOS=windows    GOARCH=386      go build -ldflags '-s' -o bin/xpos-windows-386.exe    agent/main.go
GOOS=windows    GOARCH=amd64    go build -ldflags '-s' -o bin/xpos-windows-amd64.exe  agent/main.go