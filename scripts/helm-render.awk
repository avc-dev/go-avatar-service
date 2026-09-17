# Filter for `helm template` output → k8s/rendered/.
# Keeps only this chart's own resources (drops Bitnami subchart noise) and
# replaces the Secret document with a placeholder — we don't commit rendered
# credentials, not even dev defaults.
/^# Source: / {
  keep = ($3 ~ /^gophprofile\/templates\//)
  secret = (keep && $3 ~ /secret\.yaml$/)
  if (secret) {
    print "---"
    print "# Secret omitted from rendered output (contains credentials) — see " $3
    keep = 0
  } else if (keep) {
    print "---"
  }
}
keep { print }
