FROM --platform=$BUILDPLATFORM golang:1.27.3-alpine3.24 AS build

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG SERVICE
ARG TARGETOS
ARG TARGETARCH
RUN test -n "$SERVICE" && CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o /service ./services/${SERVICE}/cmd/app

FROM alpine:3.24 AS run

COPY --from=build /service /service

USER nobody

CMD ["/service"]
