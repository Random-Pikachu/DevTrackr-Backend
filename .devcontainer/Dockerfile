FROM mcr.microsoft.com/devcontainers/go:1.26-bookworm

RUN chown -R vscode:vscode /go
USER vscode

RUN go install github.com/air-verse/air@latest