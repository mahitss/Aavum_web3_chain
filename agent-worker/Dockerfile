FROM rust:1.83-bookworm AS build
WORKDIR /src
COPY Cargo.toml Cargo.lock ./
COPY src ./src
RUN cargo build --release

FROM gcr.io/distroless/cc-debian12:nonroot
COPY --from=build /src/target/release/agent-worker /agent-worker
ENV PORT=8080
EXPOSE 8080
ENTRYPOINT ["/agent-worker"]
