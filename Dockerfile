# --- Stage 1: build one static binary (templates and CSS are embedded in it) ---
FROM golang:1.27 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# modernc.org/sqlite is pure Go, so no C compiler is needed.
RUN CGO_ENABLED=0 go build -o /arenasuite .

# --- Stage 2: tiny image with just the binary ---
FROM gcr.io/distroless/static-debian12
COPY --from=build /arenasuite /arenasuite
ENV DB_PATH=/data/arenasuite.db
VOLUME ["/data"]
EXPOSE 4000
ENTRYPOINT ["/arenasuite"]
