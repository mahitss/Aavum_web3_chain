FROM node:20-alpine AS build
WORKDIR /app
ARG NEXT_PUBLIC_NEXUS_ESCROW_ADDRESS
ARG NEXT_PUBLIC_API_URL=http://localhost:8081
ARG NEXT_PUBLIC_ORCHESTRATOR_API_URL=http://localhost:8081
ARG NEXT_PUBLIC_AEVUM_RPC_URL=https://rpc-test.qday.info
ENV NEXT_PUBLIC_NEXUS_ESCROW_ADDRESS=$NEXT_PUBLIC_NEXUS_ESCROW_ADDRESS
ENV NEXT_PUBLIC_API_URL=$NEXT_PUBLIC_API_URL
ENV NEXT_PUBLIC_ORCHESTRATOR_API_URL=$NEXT_PUBLIC_ORCHESTRATOR_API_URL
ENV NEXT_PUBLIC_AEVUM_RPC_URL=$NEXT_PUBLIC_AEVUM_RPC_URL
ENV NEXT_TELEMETRY_DISABLED=1
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
RUN npm run build

FROM node:20-alpine
WORKDIR /app
ENV NODE_ENV=production
ENV NEXT_TELEMETRY_DISABLED=1
COPY --from=build /app/package.json /app/package-lock.json ./
COPY --from=build /app/node_modules ./node_modules
COPY --from=build /app/.next ./.next
COPY --from=build /app/public ./public
EXPOSE 3000
CMD ["npx", "next", "start", "-H", "0.0.0.0", "-p", "3000"]
