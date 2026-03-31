FROM node:22-alpine AS builder
WORKDIR /app
COPY . .
RUN npm config set registry https://registry.npmjs.org/ \
 && npm install \
 && npm run build

FROM nginx:1.25-alpine AS runtime
COPY --from=builder /app/dist /app/dist/
COPY nginx.conf /etc/nginx/nginx.conf
EXPOSE 80
CMD ["nginx"]
