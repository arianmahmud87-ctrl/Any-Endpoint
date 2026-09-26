FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY . ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/worker-agent ./cmd/worker-agent

FROM alpine:3.22
RUN apk add --no-cache ca-certificates
COPY --from=build /out/worker-agent /usr/local/bin/worker-agent
RUN addgroup -S worker && adduser -S -G worker worker
USER worker
ENTRYPOINT ["/usr/local/bin/worker-agent"]
