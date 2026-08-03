# --- builder stage ---
FROM mcr.microsoft.com/dotnet/sdk:9.0 AS builder
WORKDIR /src

COPY src/ApiTool.Backend/ApiTool.Backend.csproj src/ApiTool.Backend/
RUN dotnet restore src/ApiTool.Backend/ApiTool.Backend.csproj

COPY src/ src/
WORKDIR /src/src/ApiTool.Backend
RUN dotnet publish -c Release -o /app/publish --no-restore

# --- runtime stage ---
FROM mcr.microsoft.com/dotnet/aspnet:9.0-alpine AS runtime
WORKDIR /app

COPY --from=builder /app/publish .

# Install wget for healthcheck; create data directory for SQLite.
# icu-libs: the alpine runtime image ships without ICU and defaults to
# globalization-invariant mode, where CultureInfo.GetCultureInfo("en-US")
# throws — which took down the billing_receipt email handler.
RUN apk add --no-cache wget icu-libs && mkdir -p data

ENV DOTNET_SYSTEM_GLOBALIZATION_INVARIANT=false
ENV ASPNETCORE_URLS=http://0.0.0.0:5000
ENV ASPNETCORE_ENVIRONMENT=Development
EXPOSE 5000

HEALTHCHECK --interval=5s --timeout=3s --retries=20 \
  CMD wget -qO- http://localhost:5000/swagger/v1/swagger.json >/dev/null 2>&1 || exit 1

ENTRYPOINT ["dotnet", "ApiTool.Backend.dll"]
