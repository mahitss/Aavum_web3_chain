FROM golang:1.22-alpine AS build
WORKDIR /src
RUN apk add --no-cache git build-base
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/agent-orchestrator cmd/main.go

FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata curl
COPY --from=build /bin/agent-orchestrator /usr/local/bin/agent-orchestrator
EXPOSE 8081
ENTRYPOINT ["agent-orchestrator"]
