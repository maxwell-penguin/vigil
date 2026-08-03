# mattn/go-sqlite3 is cgo, so a binary built the normal way is dynamically
# linked against musl and won't run on `scratch` (no libc there at all).
# Statically linking against musl on Alpine is what makes the scratch stage
# actually work.
FROM golang:1.22-alpine AS build
RUN apk add --no-cache gcc musl-dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build \
    -ldflags '-linkmode external -extldflags "-static"' \
    -o /vigil ./cmd/server

FROM scratch
WORKDIR /data
COPY --from=build /vigil /vigil
COPY vigil.yaml.example /data/vigil.yaml
EXPOSE 8080
ENTRYPOINT ["/vigil", "-config", "/data/vigil.yaml"]
