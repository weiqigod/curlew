# Multi-stage build for the fake SAML IdP sidecar.
# Stage 1: build
FROM mcr.microsoft.com/dotnet/sdk:9.0 AS build
WORKDIR /src
COPY FakeIdp.csproj .
RUN dotnet restore
COPY . .
RUN dotnet publish FakeIdp.csproj -c Release -o /app/out

# Stage 2: runtime
FROM mcr.microsoft.com/dotnet/aspnet:9.0
WORKDIR /app
COPY --from=build /app/out .
# Template files needed at runtime
COPY metadata-template.xml .
EXPOSE 8088
ENTRYPOINT ["dotnet", "FakeIdp.dll"]
