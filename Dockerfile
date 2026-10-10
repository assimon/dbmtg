FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /dbmtg ./cmd/dbmtg

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=build /dbmtg /app/dbmtg
VOLUME /app/data
EXPOSE 8080
ENTRYPOINT ["/app/dbmtg", "-c", "/app/config.yaml"]
