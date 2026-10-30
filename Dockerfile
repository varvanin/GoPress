FROM node:24.14.1 AS frontend
WORKDIR /src
COPY package.json package-lock.json ./
RUN npm ci
COPY web ./web
COPY scripts ./scripts
COPY third_party ./third_party
COPY internal ./internal
RUN npm run build

FROM golang:1.27.1 AS build
ARG RELEASE_VERSION=0.2.0-dev
ARG UPDATE_PUBLIC_KEY
ARG IMAGE_REPOSITORY=ghcr.io/varvanin/gopress
ARG UPDATE_FEED=https://github.com/varvanin/GoPress/releases/latest/download/update.json
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /src/internal/httpui/assets /src/internal/httpui/assets
RUN go run github.com/a-h/templ/cmd/templ@v0.3.1070 generate \
    && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X gopress/internal/version.Deployment=docker -X gopress/internal/version.Current=${RELEASE_VERSION} -X gopress/internal/version.UpdatePublicKey=${UPDATE_PUBLIC_KEY} -X gopress/internal/version.ImageRepository=${IMAGE_REPOSITORY} -X gopress/internal/version.UpdateFeed=${UPDATE_FEED}" -o /out/gopress ./cmd/gopress \
    && mkdir /out/data && chown 65532:65532 /out/data

FROM scratch
COPY --from=build /out/gopress /gopress
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build --chown=65532:65532 /out/data /data
USER 65532:65532
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=3s --start-period=60s --retries=3 CMD ["/gopress", "healthcheck", "--addr", "127.0.0.1:8080"]
ENTRYPOINT ["/gopress"]
CMD ["serve", "--data", "/data", "--addr", "0.0.0.0:8080"]
