# Egress proxy for the browser service: Stripe's Smokescreen
# (https://github.com/stripe/smokescreen). With no ACL file it allows public
# destinations and refuses loopback, private, link-local (169.254.169.254 cloud
# metadata) and other non-global addresses, checked on the resolved IP.
FROM golang:1.25.1-alpine AS build
RUN CGO_ENABLED=0 go install github.com/stripe/smokescreen@v0.1.0

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /go/bin/smokescreen /smokescreen
EXPOSE 4750
ENTRYPOINT ["/smokescreen"]
CMD ["--listen-ip", "0.0.0.0", "--listen-port", "4750"]
