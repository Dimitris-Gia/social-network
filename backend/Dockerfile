# Owner: B — multi-stage build (week 3)
# mattn/go-sqlite3 needs CGO, so gcc must be present in the build stage.
FROM golang:1.22-alpine AS build
RUN apk add --no-cache gcc musl-dev
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 go build -o /out/server .

FROM alpine:3.20
# sqlite so auditors can run: docker exec -it backend sqlite3 /app/data/social.db .tables
RUN apk add --no-cache sqlite
WORKDIR /app
COPY --from=build /out/server ./server
COPY pkg/db/migrations ./pkg/db/migrations
RUN mkdir -p data uploads
EXPOSE 8080
CMD ["./server"]
