import styles from "./ErrorList.module.css";

export function ErrorList({ errorCounts }: { errorCounts: Record<string, number> }) {
  const entries = Object.entries(errorCounts);
  if (entries.length === 0) return null;

  return (
    <div className={styles.wrap}>
      <div className={styles.label}>error counts</div>
      <ul className={styles.list}>
        {entries.map(([msg, count]) => (
          <li key={msg} className={styles.item}>
            <span className={styles.count}>{count}×</span> {msg}
          </li>
        ))}
      </ul>
    </div>
  );
}
