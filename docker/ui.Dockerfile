FROM node:24-alpine AS dev
WORKDIR /app
RUN npm install -g pnpm@12.6.0
COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml ./
RUN --mount=type=cache,id=pnpm-store,target=/pnpm/store \
    pnpm install --frozen-lockfile --store-dir /pnpm/store --package-import-method copy
COPY web/ .
EXPOSE 5173
CMD ["npm", "run", "dev", "--", "--host", "0.0.0.0"]

FROM node:24-alpine AS builder
WORKDIR /app
RUN npm install -g pnpm@12.6.0
COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml ./
RUN --mount=type=cache,id=pnpm-store,target=/pnpm/store \
    pnpm install --frozen-lockfile --store-dir /pnpm/store --package-import-method copy
COPY web/ .
ARG VERSION=dev
ARG GIT_SHA=unknown
ARG BUILD_TIME=
ENV VITE_APP_VERSION=${VERSION} VITE_GIT_SHA=${GIT_SHA} VITE_BUILD_TIME=${BUILD_TIME}
RUN npm run build

FROM nginx:alpine AS production
COPY --from=builder /app/dist /usr/share/nginx/html
# The nginx image renders /etc/nginx/templates/*.template at start, substituting only GALLERY_* variables.
# GALLERY_UI_MODE picks the snippet: public (no admin routes) or admin.
COPY docker/nginx/default.conf.template /etc/nginx/templates/default.conf.template
COPY docker/nginx/public.conf docker/nginx/admin.conf /etc/nginx/snippets/
ENV GALLERY_UI_MODE=public NGINX_ENVSUBST_FILTER=GALLERY_
EXPOSE 80
