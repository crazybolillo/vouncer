FROM golang:1.22-alpine

WORKDIR /app

COPY . .

ENV CGO_ENABLED=0
RUN go build -o /bin/vouncer ./cmd

FROM scratch

COPY --from=0 /bin/vouncer /bin/vouncer

ENTRYPOINT ["vouncer"]
