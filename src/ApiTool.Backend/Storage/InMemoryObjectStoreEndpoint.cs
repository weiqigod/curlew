using HttpResults = Microsoft.AspNetCore.Http.Results;

namespace ApiTool.Backend.Storage;

/// <summary>
/// Dev/Testing-only endpoint that serves objects from <see cref="InMemoryObjectStore"/> so that
/// signed URL round-trips work in integration tests without real cloud storage.
/// Registered only when <c>ApiTool:ObjectStore:Provider == "in_memory"</c> and environment
/// is Development or Testing.
/// </summary>
public static class InMemoryObjectStoreEndpoint
{
    /// <summary>
    /// Conditionally maps <c>GET /api/v1/internal/object-store/{*key}</c> to serve
    /// in-memory blobs. Only wired in Development and Testing environments.
    /// </summary>
    public static IEndpointRouteBuilder MapInMemoryObjectStoreEndpoints(
        this IEndpointRouteBuilder app,
        IWebHostEnvironment env,
        IObjectStore store)
    {
        if (!env.IsDevelopment() && !env.IsEnvironment("Testing"))
            return app;

        if (store is not InMemoryObjectStore)
            return app;

        app.MapGet("/api/v1/internal/object-store/{**key}", async (
            string key,
            CancellationToken ct) =>
        {
            try
            {
                var inMemory = (InMemoryObjectStore)store;
                var stream = await inMemory.GetAsync(key, ct);
                return HttpResults.Stream(stream, "application/json");
            }
            catch (KeyNotFoundException)
            {
                return HttpResults.NotFound();
            }
        })
        .AllowAnonymous()
        .DisableAntiforgery()
        .WithName("InMemoryObjectStoreGet")
        .WithTags("Internal");

        return app;
    }
}
