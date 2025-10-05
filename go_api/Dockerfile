FROM golang:1.23-alpine AS build
WORKDIR /app
RUN apk add --no-cache protobuf git build-base
# Copy module file early to leverage caching and download deps
COPY go.mod ./
RUN go mod download
# Install protoc plugins
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@latest && \
    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
# Copy the rest of the source
COPY . .
# Generate protobuf stubs
RUN mkdir -p internal/pb && \
    protoc \
      --go_out=internal/pb --go_opt=paths=source_relative \
      --go-grpc_out=internal/pb --go-grpc_opt=paths=source_relative \
      protos/fraud_detection.proto
# Ensure dependencies and go.sum are present
RUN go mod tidy
# Build binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/go-api main.go

FROM alpine:3.20
WORKDIR /app
COPY --from=build /out/go-api /usr/local/bin/go-api
EXPOSE 8000
ENTRYPOINT ["/usr/local/bin/go-api"]


