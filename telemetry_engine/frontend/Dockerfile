# Build the Vite SPA, then serve the static bundle with nginx. nginx also
# reverse-proxies /v1 (REST + WebSocket) to the leaderboard-service so the
# browser sees a single origin — no CORS, and WS upgrades work transparently.

FROM node:20-alpine AS build
WORKDIR /app
COPY package.json package-lock.json* ./
RUN npm install
COPY . .
RUN npm run build

FROM nginx:1.27-alpine AS runtime
COPY --from=build /app/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80
