# syntax=docker/dockerfile:1
#
# Frontend image: Angular 21 with server-side rendering on Node.
# Build context is the frontend/ directory:
#   docker build -t steamscope-web --build-arg API_URL=https://api.example.com/api frontend

FROM node:22-alpine AS build
WORKDIR /app

COPY package.json package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci

COPY . .
# Bake the API address into the production environment file.
ARG API_URL=http://localhost:8080/api
RUN printf "export const environment = {\n  apiUrl: '%s',\n};\n" "$API_URL" > src/environments/environment.production.ts \
    && npx ng build

FROM node:22-alpine
WORKDIR /app
ENV NODE_ENV=production PORT=4000

# The SSR bundle imports express (and friends) at runtime.
COPY package.json package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci --omit=dev
COPY --from=build /app/dist/frontend ./dist/frontend

USER node
EXPOSE 4000
HEALTHCHECK --interval=30s --timeout=3s --start-period=20s --retries=3 \
  CMD wget -qO- http://127.0.0.1:${PORT}/ >/dev/null || exit 1

# Angular rejects requests for unknown Host headers. Set this to your public
# hostname(s), comma separated:  -e NG_ALLOWED_HOSTS=app.example.com
CMD ["node", "dist/frontend/server/server.mjs"]
