using FluentAssertions;

namespace ApiTool.Backend.Tests;

public sealed class SmokeTests
{
    [Fact]
    public void Program_type_is_discoverable_for_WebApplicationFactory()
    {
        typeof(Program).Should().NotBeNull();
    }
}
