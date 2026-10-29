# syntax=docker/dockerfile:1.7

FROM --platform=$BUILDPLATFORM node:25-alpine AS build
ENV COREPACK_ENABLE_DOWNLOAD_PROMPT=0
RUN corepack enable
WORKDIR /app
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile
COPY . .
RUN pnpm build

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
