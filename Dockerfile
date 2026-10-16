# --- Stage 1: build one static binary (templates and CSS are embedded in it) ---
FROM golang:1.27 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /arenasuite .

# --- Stage 2: tiny image with just the binary ---
FROM gcr.io/distroless/static-debian12
COPY --from=build /arenasuite /arenasuite
# Settings (DATABASE_URL, SESSION_SECRET, ...) come from the host's environment.
EXPOSE 4000
ENTRYPOINT ["/arenasuite"]
