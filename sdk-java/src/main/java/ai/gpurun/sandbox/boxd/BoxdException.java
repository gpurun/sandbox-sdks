package ai.gpurun.sandbox.boxd;

public class BoxdException extends RuntimeException {
    private final int status;
    private final String code;

    public BoxdException(int status, String code, String message) { this(status, code, message, null); }
    public BoxdException(int status, String code, String message, Throwable cause) {
        super(message, cause);
        this.status = status;
        this.code = code;
    }
    public int getStatus() { return status; }
    public int status() { return status; }
    public String getCode() { return code; }
    public String code() { return code; }
}
