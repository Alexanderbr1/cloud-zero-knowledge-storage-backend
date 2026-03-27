FROM golang:1.26.1 AS build

WORKDIR /src

# Prevent implicit toolchain downloads; use the image toolchain.
ENV GOTOOLCHAIN=local

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/app ./cmd/app

FROM gcr.io/distroless/static-debian12

WORKDIR /
COPY --from=build /out/app /app

EXPOSE 8080
ENTRYPOINT ["/app"]
