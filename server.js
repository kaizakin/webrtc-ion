const express = require("express")
const app = express()

const PORT = 3000
const http = require("http")

const server = http.createServer(app)

app.use(express.static(__dirname + "/public"))

server.listen(PORT, console.log(`server listening and ready to serve on localhost:${PORT}`))
