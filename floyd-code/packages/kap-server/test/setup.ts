for (const key of Object.keys(process.env)) {
  if (key.startsWith('FLOYD_CODE_')) {
    delete process.env[key];
  }
}

process.env['FLOYD_CODE_SEARCH_WORKER'] = 'false';

process.env['FLOYD_CODE_PERSISTENCE_MINIDB_READMODEL'] = 'false';
