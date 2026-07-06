# syntax=docker/dockerfile:1

# --- build stage ---
FROM golang:1.26 AS build
WORKDIR /src

# Dependencies (cache layer)
COPY go.mod go.sum ./
RUN go mod download

# Sources
COPY . .

# SQLite is pure-Go (modernc.org/sqlite): static binary without cgo.
ENV CGO_ENABLED=0 GOOS=linux
RUN go build -trimpath -ldflags="-s -w" -o /expense_monitor .

# --- runtime stage ---
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /expense_monitor /usr/local/bin/expense_monitor

EXPOSE 7777
ENTRYPOINT ["/usr/local/bin/expense_monitor"]
CMD ["serve", "--config", "/config.yaml"]
