FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/recipes ./cmd/recipes/

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /bin/recipes /bin/recipes
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/bin/recipes"]
