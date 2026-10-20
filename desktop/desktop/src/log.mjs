import fs from 'node:fs';
import path from 'node:path';

const MAX_BYTES = 5 * 1024 * 1024;

export class Log {
  constructor(logDir) {
    this.dir = logDir;
    this.file = path.join(logDir, 'desktop.log');
    try {
      fs.mkdirSync(logDir, { recursive: true, mode: 0o700 });
    } catch {
      // Logging must never take the app down.
    }
  }

  write(level, message, extra) {
    const suffix = extra === undefined ? '' : ` ${safeJson(extra)}`;
    const line = `${new Date().toISOString()} ${level} ${message}${suffix}\n`;
    try {
      this.rotate();
      fs.appendFileSync(this.file, line, { mode: 0o600 });
    } catch {
      // Ignore write failures.
    }
    if (process.env.MEGAFLOYD_DEBUG === '1') process.stderr.write(line);
  }

  info(message, extra) {
    this.write('INFO', message, extra);
  }

  warn(message, extra) {
    this.write('WARN', message, extra);
  }

  error(message, extra) {
    this.write('ERROR', message, extra);
  }

  rotate() {
    try {
      const stat = fs.statSync(this.file);
      if (stat.size > MAX_BYTES) fs.renameSync(this.file, `${this.file}.1`);
    } catch {
      // File does not exist yet.
    }
  }
}

function safeJson(value) {
  try {
    return JSON.stringify(value);
  } catch {
    return '"<unserializable>"';
  }
}
