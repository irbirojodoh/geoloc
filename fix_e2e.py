import re
with open("e2e_test.go", "r") as f:
    s = f.read()

s = s.replace("package e2e", "package handlers")
s = s.replace("data.data.", "data.")
s = s.replace("auth.auth.", "auth.")
s = s.replace("handlers.", "")

# Remove the extra imports block at the top
s = re.sub(r'import \(\n\t"social-geo-go/internal/auth"\n\t"social-geo-go/internal/data"\n\t"social-geo-go/internal/handlers"\n\n', 'import (\n', s)

with open("e2e_test.go", "w") as f:
    f.write(s)
