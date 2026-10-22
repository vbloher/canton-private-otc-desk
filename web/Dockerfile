# syntax=docker/dockerfile:1.7

# The build output is static, so it is built once on the builder's own platform:
# a multi-platform image needs no emulation for this stage.
FROM --platform=$BUILDPLATFORM node:24-alpine AS build
ENV COREPACK_ENABLE_DOWNLOAD_PROMPT=0
RUN corepack enable
WORKDIR /app
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile
COPY . .
RUN pnpm build

# nginx serves it and proxies /api/ (and the /api/v1/ws upgrade) to the gateway.
# The config is rendered at start from docker/nginx.conf.template:
#   NGINX_PORT        port to listen on
#   NGINX_HOST        address to listen on; empty = every interface
#   GATEWAY_UPSTREAM  host:port of the gateway
# The resolver comes from the container's /etc/resolv.conf (NGINX_ENTRYPOINT_LOCAL_RESOLVERS),
# and envsubst touches only the variables the filter names, never nginx's own.
FROM nginx:1.30-alpine
ENV NGINX_PORT=80 \
    NGINX_HOST= \
    GATEWAY_UPSTREAM=gateway:8080 \
    NGINX_ENTRYPOINT_LOCAL_RESOLVERS=1 \
    NGINX_ENVSUBST_FILTER="^(NGINX_LISTEN|NGINX_LOCAL_RESOLVERS|GATEWAY_UPSTREAM)$"
COPY --chmod=755 docker/listen.envsh /docker-entrypoint.d/16-otc-listen.envsh
COPY docker/nginx.conf.template /etc/nginx/templates/default.conf.template
COPY --from=build /app/dist /usr/share/nginx/html
EXPOSE 80
