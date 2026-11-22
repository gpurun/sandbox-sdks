package ai.gpurun.sandbox.core;

@Deprecated
public class Novita extends Gpurun {
    private Novita(SandboxOpts options) throws SandboxException { super(options); }

    public static Novita create() throws SandboxException { return new Novita(new SandboxOptions()); }

    public static Novita create(SandboxOptions options) throws SandboxException {
        return new Novita(options == null ? new SandboxOptions() : options);
    }

    public static Novita create(SandboxOpts options) throws SandboxException {
        return new Novita(options == null ? new SandboxOpts() : options);
    }
}
