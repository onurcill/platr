# context: grpc-inspector/
FROM node:20-alpine AS builder

WORKDIR /src
COPY frontend/package*.json ./
RUN npm ci --frozen-lockfile

COPY frontend/ .
RUN npm run build

FROM nginx:1.27-alpine
COPY --from=builder /src/dist /usr/share/nginx/html
COPY deploy/frontend/nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]
