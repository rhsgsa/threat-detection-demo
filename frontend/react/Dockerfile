FROM docker.io/node:16

EXPOSE 3000
CMD ["npm", "start"]
WORKDIR /app
COPY . /app
RUN npm install .
