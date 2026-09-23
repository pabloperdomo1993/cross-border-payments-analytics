# --- build stage -----------------------------------------------------
FROM node:20-alpine AS builder

WORKDIR /src

COPY package.json package-lock.json ./
RUN npm ci

COPY . .
RUN npm run build

# --- runtime stage -----------------------------------------------------
# No Node.js toolchain in the final image: the build output is static
# files, served by a minimal web server.
FROM nginx:1.27-alpine

COPY --from=builder /src/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf

EXPOSE 80
