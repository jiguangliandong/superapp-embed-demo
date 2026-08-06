import { Router } from "express";

import { config } from "../config.js";

const router = Router();

router.get("/", (_request, response) => {
  response.redirect("/app");
});

router.get("/app", (request, response) => {
  const launchId =
    typeof request.query.launch_id === "string"
      ? request.query.launch_id
      : "";

  response.set("Cache-Control", "no-store");
  response.render("app", {
    title: "SuperApp H5 SSO Demo",
    clientId: config.clientId,
    hasLaunchContext: launchId.startsWith("lnch_"),
  });
});

router.get("/privacy", (_request, response) => {
  response.render("privacy", {
    title: "隐私政策",
  });
});

export default router;
