# Legacy single-binary image. Production uses `encore build docker` with
# deploy/encore-infra.json; see docs/encore-migration.md.
FROM node:24.12.0-alpine AS frontend
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

FROM golang:1.27.3 AS build
WORKDIR /src
COPY go.mod go.sum ./
COPY third_party/simpleiot/go.mod third_party/simpleiot/go.sum ./third_party/simpleiot/
RUN go mod download
COPY . .
COPY --from=frontend /src/internal/web/dist ./internal/web/dist
RUN CGO_ENABLED=0 go build -trimpath -o /out/fastiot ./cmd/fastiot

FROM scratch
COPY --from=build /out/fastiot /fastiot
VOLUME ["/data"]
EXPOSE 8080
ENTRYPOINT ["/fastiot", "-data", "/data"]
