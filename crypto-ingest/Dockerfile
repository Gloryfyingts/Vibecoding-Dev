FROM golang:1.22.12-alpine3.21 AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /crypto-ingest ./cmd/ingest

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /crypto-ingest /crypto-ingest

ENTRYPOINT ["/crypto-ingest"]
