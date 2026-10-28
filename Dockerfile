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
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /src/internal/httpui/assets /src/internal/httpui/assets
RUN go run github.com/a-h/templ/cmd/templ@v0.3.1070 generate \
    && CGO_ENABLED=0 go build -trimpath -o /out/gopress ./cmd/gopress \
    && mkdir /out/data && chown 65532:65532 /out/data

FROM scratch
COPY --from=build /out/gopress /gopress
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build --chown=65532:65532 /out/data /data
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/gopress"]
CMD ["serve", "--data", "/data", "--addr", "0.0.0.0:8080"]
