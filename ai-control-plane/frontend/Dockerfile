FROM node:22-alpine AS build
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci --ignore-scripts
COPY . .
ARG VITE_PREVIEW_MODE=false
ENV VITE_PREVIEW_MODE=${VITE_PREVIEW_MODE}
RUN npm run build

FROM alpine:3.22
COPY --from=build /app/dist /build/dist
WORKDIR /build
