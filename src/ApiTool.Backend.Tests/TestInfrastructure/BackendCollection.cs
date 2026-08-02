using ApiTool.Backend.Tests.TestInfrastructure;

namespace ApiTool.Backend.Tests.TestInfrastructure;

/// <summary>
/// xUnit collection fixture — groups all integration tests that share a
/// <see cref="BackendFactory"/> into one sequential run to avoid SQLite
/// concurrency conflicts within the shared in-memory database.
/// </summary>
[CollectionDefinition(Name)]
public sealed class BackendCollection : ICollectionFixture<BackendFactory>
{
    /// <summary>Collection name used in <see cref="CollectionAttribute"/>.</summary>
    public const string Name = "Backend";
}
