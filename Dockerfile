FROM golang:1.26.1 AS build

WORKDIR /src

ENV GOTOOLCHAIN=local

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/app ./cmd/app
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/migrate ./cmd/migrate

FROM gcr.io/distroless/static-debian12 AS app

WORKDIR /
COPY --from=build /out/app /app
EXPOSE 8080
ENTRYPOINT ["/app"]

FROM gcr.io/distroless/static-debian12 AS migrate

WORKDIR /
COPY --from=build /out/migrate /migrate
ENTRYPOINT ["/migrate"]
