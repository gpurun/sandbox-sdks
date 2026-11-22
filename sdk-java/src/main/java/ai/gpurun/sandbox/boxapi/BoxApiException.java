package ai.gpurun.sandbox.boxapi;

public class BoxApiException extends RuntimeException {
    private final int status;
    private final String code;

    public BoxApiException(int status, String code, String message) { this(status, code, message, null); }
    public BoxApiException(int status, String code, String message, Throwable cause) {
        super(message, cause);
        this.status = status;
        this.code = code;
    }
    public int getStatus() { return status; }
    public int status() { return status; }
    public String getCode() { return code; }
    public String code() { return code; }
}
