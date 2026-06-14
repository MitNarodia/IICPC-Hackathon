FROM golang:1.25-alpine AS build
RUN apk add --no-cache git
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY pkg/ pkg/
COPY services/upload-service/ services/upload-service/

RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /app ./services/upload-service/cmd/upload-service

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /app /app
ENTRYPOINT ["/app"]
