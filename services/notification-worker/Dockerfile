FROM node:20-alpine

RUN apk add --no-cache git

WORKDIR /app

COPY package*.json ./
RUN npm install

COPY . .

EXPOSE 9090

CMD ["npm", "start"]
