"""Use the installed speech and music engines without shared final outputs."""
import base64
import binascii
import json
import re
import subprocess
import tempfile
import threading
import time
import urllib.parse
import urllib.request
import uuid
from pathlib import Path


SPEECH_MODEL = '/home/douglas/models-library/imported/handy-computer/nemotron-3.5-asr-streaming-0.6b-gguf/nemotron-3.5-asr-streaming-0.6b-F32.gguf'
SCHEMAS = {
    'speech-transcribe': {'name': 'Turn a recording into text', 'inputSchema': {
        'type': 'object', 'properties': {'audio_base64': {'type': 'string'},
            'filename': {'type': 'string'}, 'language': {'type': 'string', 'default': 'en-US'}},
        'required': ['audio_base64', 'filename']}},
    'music-create': {'name': 'Make music', 'inputSchema': {
        'type': 'object', 'properties': {'caption': {'type': 'string'}, 'lyrics': {'type': 'string'},
            'seconds': {'type': 'number', 'minimum': 5, 'maximum': 60, 'default': 15},
            'seed': {'type': 'integer', 'minimum': 0, 'maximum': 4294967295},
            'destination': {'type': 'string', 'description': 'The output file on your own computer.'}},
        'required': ['caption', 'destination']}}
}


class MediaTools:
    def __init__(self, platform):
        self.platform = platform
        self.speech_capacity = threading.BoundedSemaphore(2)
        self.music_capacity = threading.BoundedSemaphore(1)

    def initialize(self, conn):
        conn.execute('CREATE TABLE IF NOT EXISTS media_jobs(id TEXT PRIMARY KEY,user_id TEXT NOT NULL,'
                     'created_at INTEGER NOT NULL,status TEXT NOT NULL,prompt_id TEXT,output_json TEXT,error TEXT)')
        conn.execute("UPDATE media_jobs SET status='failed',error='The service restarted before the recording was delivered.' WHERE status='working'")
        for id_, schema in SCHEMAS.items():
            conn.execute('INSERT OR IGNORE INTO shared_tools(id,name,server,tool,schema_json) VALUES(?,?,?,?,?)',
                         (id_, schema['name'], 'company-media', id_, json.dumps(schema)))

    def call(self, user, id_, arguments):
        if id_ == 'speech-transcribe':
            return self.transcribe(arguments)
        if id_ == 'music-create':
            return self.start_music(user, arguments)
        self.platform.fail(404, 'That company tool is not available.')

    def transcribe(self, args):
        filename = args.get('filename', '')
        encoded = args.get('audio_base64')
        language = args.get('language', 'en-US')
        if not isinstance(filename, str) or not re.fullmatch(r'[A-Za-z0-9_. -]{1,160}', filename) or filename.startswith('.'):
            self.platform.fail(400, 'Use a recording filename without a folder name.')
        suffix = Path(filename).suffix.lower()
        if suffix not in ['.wav', '.mp3', '.m4a', '.ogg', '.flac']:
            self.platform.fail(400, 'Choose a WAV, MP3, M4A, OGG or FLAC recording.')
        if not isinstance(encoded, str) or len(encoded) > 30 * 1024 * 1024:
            self.platform.fail(400, 'Choose a recording smaller than 22 MB.')
        try:
            audio = base64.b64decode(encoded, validate=True)
        except (ValueError, binascii.Error):
            self.platform.fail(400, 'The recording was not received correctly.')
        if not audio or len(audio) > 22 * 1024 * 1024:
            self.platform.fail(400, 'Choose a recording smaller than 22 MB.')
        if not isinstance(language, str) or not re.fullmatch(r'[a-zA-Z]{2,3}(?:-[a-zA-Z]{2,4})?', language):
            self.platform.fail(400, 'Choose a language such as en-US.')
        if not self.speech_capacity.acquire(blocking=False):
            self.platform.fail(429, 'Recording tools are busy. Try again shortly.')
        try:
            with tempfile.TemporaryDirectory(prefix='floyd-recording-') as folder:
                folder = Path(folder)
                original, wav, output = folder / ('input' + suffix), folder / 'recording.wav', folder / 'text.txt'
                original.write_bytes(audio)
                subprocess.run(['/usr/bin/ffmpeg', '-nostdin', '-v', 'error', '-protocol_whitelist', 'file,pipe',
                    '-i', str(original), '-t', '300', '-ac', '1', '-ar', '16000', '-y', str(wav)],
                    check=True, timeout=15, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
                subprocess.run(['/srv/organized/services/floyd-transcribe/bin/transcribe-cli', '-q',
                    '--backend', 'cpu', '--threads', '6', '--timestamps', 'none', '--language', language,
                    '-m', SPEECH_MODEL, '--output', str(output), str(wav)], check=True, timeout=45,
                    stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
                text = output.read_text().strip()
                return {'text': text, 'model': 'Nemotron 3.5 Speech', 'truncated_after_seconds': 300}
        except (OSError, subprocess.SubprocessError):
            self.platform.fail(502, 'The recording could not be read. Try a shorter recording.')
        finally:
            self.speech_capacity.release()

    def comfy(self, path, body=None):
        request = urllib.request.Request('http://127.0.0.1:8188' + path,
            data=json.dumps(body).encode() if body is not None else None,
            headers={'Content-Type': 'application/json'})
        with urllib.request.urlopen(request, timeout=8) as response:
            return json.load(response)

    def start_music(self, user, args):
        destination = args.get('destination')
        if not isinstance(destination, str) or not destination.strip() or len(destination) > 4000:
            self.platform.fail(400, 'No destination folder specified. Supply an output path within your user space.')
        caption, lyrics = args.get('caption'), args.get('lyrics', '')
        seconds, seed = args.get('seconds', 15), args.get('seed', 7)
        if not isinstance(caption, str) or not caption.strip() or len(caption) > 4000 or not isinstance(lyrics, str) or len(lyrics) > 12000:
            self.platform.fail(400, 'Describe the music and keep the words short.')
        if isinstance(seconds, bool) or not isinstance(seconds, (int, float)) or not 5 <= seconds <= 60:
            self.platform.fail(400, 'Choose a length from 5 to 60 seconds.')
        if isinstance(seed, bool) or not isinstance(seed, int) or not 0 <= seed <= 4294967295:
            self.platform.fail(400, 'Choose a whole-number starting value.')
        if not self.music_capacity.acquire(blocking=False):
            self.platform.fail(429, 'Music is being made for another request. Try again shortly.')
        id_ = uuid.uuid4().hex
        try:
            with self.platform.database() as conn:
                conn.execute('INSERT INTO media_jobs(id,user_id,created_at,status) VALUES(?,?,?,?)',
                             (id_, user['id'], self.platform.app.now(), 'working'))
            threading.Thread(target=self.make_music, args=(id_, caption, lyrics, seconds, seed), daemon=True).start()
        except Exception:
            self.music_capacity.release()
            raise
        return {'job': id_, 'status': 'working'}

    def make_music(self, id_, caption, lyrics, seconds, seed):
        prompt_id = None
        try:
            graph = {
                '1': {'class_type': 'UnetLoaderGGUF', 'inputs': {'unet_name': 'MiniMax-Music3-F16.gguf'}},
                '2': {'class_type': 'CLIPLoader', 'inputs': {'clip_name': 'minimax_music3_text_encoder_pruned_int8_convrot.safetensors', 'type': 'minimax'}},
                '3': {'class_type': 'VAELoader', 'inputs': {'vae_name': 'minimax_music3_dav.safetensors'}},
                '5': {'class_type': 'MiniMaxMusic3TextEncode', 'inputs': {'clip': ['2', 0], 'caption': caption,
                    'lyrics': lyrics, 'seed': seed, 'max_duration': seconds, 'cfg_scale': 1.7, 'top_k': 50}},
                '6': {'class_type': 'EmptyMiniMaxMusic3LatentAudio', 'inputs': {'seconds': ['5', 1], 'batch_size': 1}},
                '7': {'class_type': 'KSampler', 'inputs': {'model': ['1', 0], 'seed': seed, 'steps': 30,
                    'cfg': 1.7, 'sampler_name': 'euler', 'scheduler': 'simple', 'positive': ['5', 0],
                    'negative': ['5', 0], 'latent_image': ['6', 0], 'denoise': 1.0}},
                '8': {'class_type': 'VAEDecodeAudio', 'inputs': {'samples': ['7', 0], 'vae': ['3', 0]}},
                '9': {'class_type': 'PreviewAudio', 'inputs': {'audio': ['8', 0]}}
            }
            submitted = self.comfy('/prompt', {'prompt': graph, 'client_id': 'floyd-company-' + id_})
            prompt_id = submitted['prompt_id']
            with self.platform.database() as conn:
                conn.execute('UPDATE media_jobs SET prompt_id=? WHERE id=?', (prompt_id, id_))
            deadline = time.monotonic() + 900
            while time.monotonic() < deadline:
                history = self.comfy('/history/' + prompt_id).get(prompt_id)
                if history:
                    if history.get('status', {}).get('status_str') == 'error':
                        raise RuntimeError(json.dumps(history.get('status', {}).get('messages', []))[:2000])
                    rows = history.get('outputs', {}).get('9', {}).get('audio', [])
                    if rows:
                        saved = rows[0]
                        name, subfolder = saved['filename'], saved.get('subfolder', '')
                        scratch = (Path('/opt/ComfyUI/temp') / subfolder / name).resolve()
                        if saved.get('type') != 'temp' or not scratch.is_relative_to(Path('/opt/ComfyUI/temp').resolve()):
                            raise RuntimeError('The music engine returned an unexpected output folder.')
                        with urllib.request.urlopen('http://127.0.0.1:8188/view?' + urllib.parse.urlencode(saved), timeout=8) as response:
                            audio = response.read(16 * 1024 * 1024 + 1)
                        if not audio or len(audio) > 16 * 1024 * 1024:
                            raise RuntimeError('The music engine returned no complete recording.')
                        result = {'audio_base64': base64.b64encode(audio).decode(), 'format': 'flac',
                                  'model': 'MiniMax Music3', 'bytes': len(audio)}
                        with self.platform.database() as conn:
                            conn.execute("UPDATE media_jobs SET status='complete',output_json=? WHERE id=?", (json.dumps(result), id_))
                        scratch.unlink(missing_ok=True)
                        return
                time.sleep(2)
            raise TimeoutError('The music request exceeded 15 minutes.')
        except Exception as error:
            with self.platform.database() as conn:
                conn.execute("UPDATE media_jobs SET status='failed',error=? WHERE id=?", (str(error)[:2000], id_))
        finally:
            if prompt_id:
                try:
                    self.comfy('/queue', {'delete': [prompt_id]})
                    self.comfy('/history', {'delete': [prompt_id]})
                except (OSError, ValueError):
                    pass
            self.music_capacity.release()

    def route(self, handler, method, path):
        if not path.startswith('/company/media/jobs/'):
            return None
        with self.platform.database() as conn:
            user = self.platform.app.require_bearer_user(conn, handler.headers)
            row = conn.execute('SELECT * FROM media_jobs WHERE id=? AND user_id=?', (path.rsplit('/', 1)[-1], user['id'])).fetchone()
        if row is None:
            self.platform.fail(404, 'That recording request was not found for your account.')
        if method != 'GET':
            self.platform.fail(405, 'Read the recording request to check its progress.')
        if row['status'] == 'failed':
            return 200, {'job': row['id'], 'status': 'failed', 'message': 'The music could not be made. Try again shortly.'}, 'json', {}
        return 200, {'job': row['id'], 'status': row['status'],
                     **(json.loads(row['output_json']) if row['output_json'] else {})}, 'json', {}

    def cleanup(self):
        with self.platform.database() as conn:
            conn.execute('DELETE FROM media_jobs WHERE created_at<?', (self.platform.app.now() - 3600,))
