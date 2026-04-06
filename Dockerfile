# Stamped HTML + CSS (roadmap 49: versioned asset URLs for CDN/long cache)
FROM node:20-alpine AS frontend

ARG ASSET_VERSION=
ENV ASSET_VERSION=${ASSET_VERSION}

WORKDIR /frontend

COPY package.json package-lock.json tailwind.config.js postcss.config.js ./
COPY src ./src
COPY scripts ./scripts
COPY *.html ./

RUN npm ci \
	&& npm run build:css \
	&& node scripts/stamp-frontend-assets.cjs

# Go binary (keep in sync with go.mod / CI Go version)
FROM golang:1.24 AS builder

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main ./cmd/server

# Runtime
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/

COPY --from=builder /app/main .
COPY --from=frontend /frontend/build/stamped/ ./
COPY --from=frontend /frontend/dist ./dist
COPY --from=builder /app/static ./static
COPY --from=builder /app/images ./images

EXPOSE 3000

USER nobody

CMD ["./main"]
