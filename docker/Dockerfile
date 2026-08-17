# ---- build stage ----
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/chaos-api ./cmd/server

# ---- runtime stage ----
FROM alpine:3.20
RUN adduser -D -u 10001 apiuser
WORKDIR /app
COPY --from=build /out/chaos-api .
COPY migrations ./migrations

# Create the upload directory here, owned by the runtime user, BEFORE dropping
# privileges. A named volume mounted over this path inherits the ownership of
# the image's directory when Docker first creates it — if the path does not
# exist in the image, Docker makes it root-owned and every avatar upload fails
# with permission denied on a fresh deploy.
RUN mkdir -p /app/uploads && chown apiuser:apiuser /app/uploads

USER apiuser
EXPOSE 8080
ENTRYPOINT ["./chaos-api"]
