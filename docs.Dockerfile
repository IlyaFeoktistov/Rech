# Собирает пользовательскую документацию (MkDocs, docs/*.md + README.md,
# см. mkdocs.yml) и раздаёт её nginx'ом - отдельный образ от самого
# компилятора (тот - Go, этот - Python/MkDocs), см. RELEASING.md про CI,
# который его собирает и пушит.
FROM python:3.12-slim AS build
WORKDIR /src
COPY requirements-docs.txt ./
RUN pip install --no-cache-dir -r requirements-docs.txt
COPY mkdocs.yml README.md ./
COPY docs ./docs
RUN mkdocs build

FROM nginx:1.27-alpine AS runtime
COPY --from=build /src/site /usr/share/nginx/html
EXPOSE 80
