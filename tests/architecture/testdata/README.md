# Dependency Fixtures

The `.go.txt` files are parser inputs, not compiled packages. Each file names its intended owning package in the fixture
inventory in `imports_test.go`. Accepted and rejected cases prove the checker itself.
