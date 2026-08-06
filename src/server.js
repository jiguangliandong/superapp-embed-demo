import "dotenv/config";

import path from "node:path";
import { fileURLToPath } from "node:url";

import express from "express";
import session from "express-session";

import pageRoutes from "./routes/pages.js";
import ssoRoutes from "./routes/sso.js";

const projectRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);

const port = Number.parseInt(process.env.PORT ?? "3000", 10);
const sessionSecret = process.env.SESSION_SECRET;

if (!sessionSecret) {
  throw new Error("SESSION_SECRET is required");
}

const app = express();

app.set("trust proxy", 1);
app.set("view engine", "ejs");
app.set("views", path.join(projectRoot, "views"));

app.use(express.urlencoded({ extended: false }));
app.use(express.json());
app.use(express.static(path.join(projectRoot, "public")));

app.use(
  session({
    name: "partner.sid",
    secret: sessionSecret,
    resave: false,
    saveUninitialized: false,
    proxy: true,
    cookie: {
      httpOnly: true,
      sameSite: "lax",
      secure: "auto",
      maxAge: 30 * 60 * 1000,
    },
  }),
);

app.use(ssoRoutes);
app.use(pageRoutes);

app.get("/healthz", (_request, response) => {
  response.json({
    status: "ok",
    service: "superapp-h5-sso-demo",
  });
});

app.listen(port, "0.0.0.0", () => {
  console.log(`Partner H5 listening on http://localhost:${port}`);
});
