FROM --platform=$BUILDPLATFORM golang:1.27.3-alpine3.24 AS build

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o /maker-bot ./cmd/maker-bot

FROM alpine:3.24 AS run

COPY --from=build /maker-bot /maker-bot

RUN mkdir /data && chown 65534:65534 /data

USER nobody

ENV STATE_FILE=/data/identity.json

VOLUME /data

CMD ["/maker-bot"]
