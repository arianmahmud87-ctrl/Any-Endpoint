FROM golang:1.26-alpine AS build
WORKDIR /src
COPY backend/go.mod ./backend/
COPY backend ./backend
WORKDIR /src/backend
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/control-plane ./cmd/control-plane

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/control-plane /control-plane
USER nonroot:nonroot
EXPOSE 8080 8081
ENTRYPOINT ["/control-plane"]
