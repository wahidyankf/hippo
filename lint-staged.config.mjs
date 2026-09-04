export default {
  "**/*.go": ["go tool goimports -w", "go tool gofumpt -w"],
  "**/*.{json,md,yaml,yml}": "prettier --write --ignore-unknown",
  "**/*.sh": "go tool shfmt -w",
  hippo: "go tool shfmt -w",
  ".husky/{commit-msg,pre-commit,pre-push}": "go tool shfmt -w",
};
