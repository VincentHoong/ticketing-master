FROM golang:1.26-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /out/api . \
    && CGO_ENABLED=0 go build -o /out/migrate ./cmd/migrate \
    && CGO_ENABLED=0 go build -o /out/seeder ./cmd/seed \
    && CGO_ENABLED=0 go build -o /out/simulate ./cmd/simulate

FROM alpine:3.20

RUN adduser -D -u 10001 app
WORKDIR /app

COPY --from=build /out/ /app/
COPY --from=build /src/migrations /app/migrations
COPY --from=build /src/seed /app/seed

USER app
EXPOSE 8000

CMD ["/app/api"]
