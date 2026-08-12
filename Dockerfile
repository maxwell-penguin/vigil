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
COPY vigil.yaml.example /etc/vigil/vigil.yaml
EXPOSE 8080
ENTRYPOINT ["/vigil", "-config", "/etc/vigil/vigil.yaml"]
