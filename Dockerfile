FROM golang:1.24-alpine AS builder

WORKDIR /app
COPY go.mod ./
COPY . .

# We can safely run tidy using local module mappings 
RUN apk add --no-cache git
RUN go get github.com/jonas747/dca
RUN go mod tidy
RUN CGO_ENABLED=0 GOOS=linux go build -o music-binary main.go

FROM alpine:latest

WORKDIR /app
COPY --from=builder /app/music-binary .

# Important: FFMPEG and Python3 (for yt-dlp) are strictly required globally mapping
RUN apk add --no-cache tzdata ca-certificates ffmpeg python3 py3-pip
# Install yt-dlp natively bridging Python proxy locally
RUN python3 -m venv /opt/venv
ENV PATH="/opt/venv/bin:$PATH"
RUN pip install -U yt-dlp

ENTRYPOINT ["./music-binary"]
