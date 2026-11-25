# The Go server: public API on :8080, admin API on :8081.
FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY cmd ./cmd
COPY internal ./internal
ARG VERSION=dev
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/photo-gallery ./cmd/photo-gallery

FROM alpine:3.22 AS production
RUN apk add --no-cache ca-certificates tzdata \
 && addgroup -g 568 gallery && adduser -u 568 -G gallery -H -D -s /sbin/nologin gallery
COPY --from=build /out/photo-gallery /usr/local/bin/photo-gallery
ENV GALLERY_PORT=8080 GALLERY_ADMIN_PORT=8081
USER 568:568
EXPOSE 8080 8081
ENTRYPOINT ["photo-gallery"]
CMD ["serve"]
