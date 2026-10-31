export function Wordmark({ size = 16 }: { size?: number }) {
  return (
    <div className="wordmark">
      <span className="wordmark__dot" aria-hidden="true" />
      <span className="wordmark__text" style={{ fontSize: size }}>
        ORCHIS
      </span>
    </div>
  );
}
