FROM golang:1.22-alpine AS build
WORKDIR /app
# Pre-copy module files and source to allow tidy to generate go.sum
COPY go.mod ./
COPY . .
# Ensure module deps and go.sum are generated
RUN go mod download && go mod tidy
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/go-processor main.go

FROM alpine:3.20
WORKDIR /app
COPY --from=build /out/go-processor /usr/local/bin/go-processor
ENTRYPOINT ["/usr/local/bin/go-processor"]


