
/* Floyd mascot: supplied arcade plate, live mood glyphs composited into the screen. */
(function () {
  if (window.__floydMascot) return;
  window.__floydMascot = true;

  var CELLS = {"default": [[1.5, 2.0], [8.5, 2.0], [2.5, 2.0], [9.5, 2.0], [3.5, 2.0], [10.5, 2.0], [4.5, 2.0], [11.5, 2.0], [0.5, 3.0], [7.5, 3.0], [1.5, 3.0], [8.5, 3.0], [2.5, 3.0], [9.5, 3.0], [3.5, 3.0], [10.5, 3.0], [4.5, 3.0], [11.5, 3.0], [5.5, 3.0], [12.5, 3.0], [0.5, 4.0], [7.5, 4.0], [1.5, 4.0], [8.5, 4.0], [4.5, 4.0], [11.5, 4.0], [5.5, 4.0], [12.5, 4.0], [2.0, 5.6], [3.0, 5.6], [4.0, 5.6], [5.0, 5.6], [6.0, 5.6], [7.0, 5.6], [8.0, 5.6], [9.0, 5.6], [10.0, 5.6], [11.0, 5.6], [3.0, 6.6], [5.0, 6.6], [7.0, 6.6], [9.0, 6.6]], "default:blink": [[0.5, 2.0], [7.5, 2.0], [1.5, 2.0], [8.5, 2.0], [2.5, 2.0], [9.5, 2.0], [3.5, 2.0], [10.5, 2.0], [4.5, 2.0], [11.5, 2.0], [5.5, 2.0], [12.5, 2.0], [2.0, 5.6], [3.0, 5.6], [4.0, 5.6], [5.0, 5.6], [6.0, 5.6], [7.0, 5.6], [8.0, 5.6], [9.0, 5.6], [10.0, 5.6], [11.0, 5.6], [3.0, 6.6], [5.0, 6.6], [7.0, 6.6], [9.0, 6.6]], "happy": [[0.5, 2.0], [7.5, 2.0], [1.5, 2.0], [8.5, 2.0], [4.5, 2.0], [11.5, 2.0], [5.5, 2.0], [12.5, 2.0], [1.5, 3.0], [8.5, 3.0], [2.5, 3.0], [9.5, 3.0], [3.5, 3.0], [10.5, 3.0], [4.5, 3.0], [11.5, 3.0], [2.5, 4.0], [9.5, 4.0], [3.5, 4.0], [10.5, 4.0], [2.0, 5.6], [3.0, 5.6], [4.0, 5.6], [5.0, 5.6], [6.0, 5.6], [7.0, 5.6], [8.0, 5.6], [9.0, 5.6], [10.0, 5.6], [11.0, 5.6], [3.0, 6.6], [5.0, 6.6], [7.0, 6.6], [9.0, 6.6]], "happy:blink": [[0.5, 2.0], [7.5, 2.0], [1.5, 2.0], [8.5, 2.0], [2.5, 2.0], [9.5, 2.0], [3.5, 2.0], [10.5, 2.0], [4.5, 2.0], [11.5, 2.0], [5.5, 2.0], [12.5, 2.0], [2.0, 5.6], [3.0, 5.6], [4.0, 5.6], [5.0, 5.6], [6.0, 5.6], [7.0, 5.6], [8.0, 5.6], [9.0, 5.6], [10.0, 5.6], [11.0, 5.6], [3.0, 6.6], [5.0, 6.6], [7.0, 6.6], [9.0, 6.6]], "excited": [[1.5, 2.0], [8.5, 2.0], [4.5, 2.0], [11.5, 2.0], [0.5, 3.0], [7.5, 3.0], [1.5, 3.0], [8.5, 3.0], [2.5, 3.0], [9.5, 3.0], [3.5, 3.0], [10.5, 3.0], [4.5, 3.0], [11.5, 3.0], [5.5, 3.0], [12.5, 3.0], [1.5, 4.0], [8.5, 4.0], [4.5, 4.0], [11.5, 4.0], [2.0, 5.6], [3.0, 5.6], [4.0, 5.6], [5.0, 5.6], [6.0, 5.6], [7.0, 5.6], [8.0, 5.6], [9.0, 5.6], [10.0, 5.6], [11.0, 5.6], [3.0, 6.6], [5.0, 6.6], [7.0, 6.6], [9.0, 6.6]], "excited:blink": [[0.5, 2.0], [7.5, 2.0], [1.5, 2.0], [8.5, 2.0], [2.5, 2.0], [9.5, 2.0], [3.5, 2.0], [10.5, 2.0], [4.5, 2.0], [11.5, 2.0], [5.5, 2.0], [12.5, 2.0], [2.0, 5.6], [3.0, 5.6], [4.0, 5.6], [5.0, 5.6], [6.0, 5.6], [7.0, 5.6], [8.0, 5.6], [9.0, 5.6], [10.0, 5.6], [11.0, 5.6], [3.0, 6.6], [5.0, 6.6], [7.0, 6.6], [9.0, 6.6]], "focused": [[1.5, 2.0], [8.5, 2.0], [2.5, 2.0], [9.5, 2.0], [3.5, 2.0], [10.5, 2.0], [4.5, 2.0], [11.5, 2.0], [0.5, 3.0], [7.5, 3.0], [1.5, 3.0], [8.5, 3.0], [2.5, 3.0], [9.5, 3.0], [3.5, 3.0], [10.5, 3.0], [4.5, 3.0], [11.5, 3.0], [5.5, 3.0], [12.5, 3.0], [0.5, 4.0], [7.5, 4.0], [1.5, 4.0], [8.5, 4.0], [4.5, 4.0], [11.5, 4.0], [5.5, 4.0], [12.5, 4.0], [2.0, 5.6], [3.0, 5.6], [4.0, 5.6], [5.0, 5.6], [6.0, 5.6], [7.0, 5.6], [8.0, 5.6], [9.0, 5.6], [10.0, 5.6], [11.0, 5.6]], "focused:blink": [[0.5, 2.0], [7.5, 2.0], [1.5, 2.0], [8.5, 2.0], [2.5, 2.0], [9.5, 2.0], [3.5, 2.0], [10.5, 2.0], [4.5, 2.0], [11.5, 2.0], [5.5, 2.0], [12.5, 2.0], [2.0, 5.6], [3.0, 5.6], [4.0, 5.6], [5.0, 5.6], [6.0, 5.6], [7.0, 5.6], [8.0, 5.6], [9.0, 5.6], [10.0, 5.6], [11.0, 5.6]], "thinking": [[1.5, 1.0], [8.5, 1.0], [2.5, 1.0], [9.5, 1.0], [3.5, 1.0], [10.5, 1.0], [4.5, 1.0], [11.5, 1.0], [4.5, 2.0], [11.5, 2.0], [5.5, 2.0], [12.5, 2.0], [2.5, 3.0], [9.5, 3.0], [3.5, 3.0], [10.5, 3.0], [4.0, 5.6], [5.0, 5.6], [6.0, 5.6], [7.0, 5.6], [8.0, 5.6], [9.0, 5.6]], "thinking:blink": [[0.5, 2.0], [7.5, 2.0], [1.5, 2.0], [8.5, 2.0], [2.5, 2.0], [9.5, 2.0], [3.5, 2.0], [10.5, 2.0], [4.5, 2.0], [11.5, 2.0], [5.5, 2.0], [12.5, 2.0], [4.0, 5.6], [5.0, 5.6], [6.0, 5.6], [7.0, 5.6], [8.0, 5.6], [9.0, 5.6]], "analyzing": [[1.5, 2.0], [8.5, 2.0], [2.5, 2.0], [9.5, 2.0], [3.5, 2.0], [10.5, 2.0], [4.5, 2.0], [11.5, 2.0], [0.5, 3.0], [7.5, 3.0], [1.5, 3.0], [8.5, 3.0], [2.5, 3.0], [9.5, 3.0], [3.5, 3.0], [10.5, 3.0], [4.5, 3.0], [11.5, 3.0], [5.5, 3.0], [12.5, 3.0], [0.5, 4.0], [7.5, 4.0], [1.5, 4.0], [8.5, 4.0], [4.5, 4.0], [11.5, 4.0], [5.5, 4.0], [12.5, 4.0], [2.0, 5.6], [3.0, 5.6], [4.0, 5.6], [5.0, 5.6], [6.0, 5.6], [7.0, 5.6], [8.0, 5.6], [9.0, 5.6], [10.0, 5.6], [11.0, 5.6], [3.0, 6.6], [5.0, 6.6], [7.0, 6.6], [9.0, 6.6]], "analyzing:blink": [[0.5, 2.0], [7.5, 2.0], [1.5, 2.0], [8.5, 2.0], [2.5, 2.0], [9.5, 2.0], [3.5, 2.0], [10.5, 2.0], [4.5, 2.0], [11.5, 2.0], [5.5, 2.0], [12.5, 2.0], [2.0, 5.6], [3.0, 5.6], [4.0, 5.6], [5.0, 5.6], [6.0, 5.6], [7.0, 5.6], [8.0, 5.6], [9.0, 5.6], [10.0, 5.6], [11.0, 5.6], [3.0, 6.6], [5.0, 6.6], [7.0, 6.6], [9.0, 6.6]], "suspicious": [[0.5, 2.0], [7.5, 2.0], [1.5, 2.0], [8.5, 2.0], [2.5, 2.0], [9.5, 2.0], [3.5, 2.0], [10.5, 2.0], [4.5, 2.0], [11.5, 2.0], [5.5, 2.0], [12.5, 2.0], [0.5, 4.0], [7.5, 4.0], [1.5, 4.0], [8.5, 4.0], [2.5, 4.0], [9.5, 4.0], [3.5, 4.0], [10.5, 4.0], [2.0, 5.6], [3.0, 5.6], [4.0, 5.6], [5.0, 5.6], [6.0, 5.6], [7.0, 5.6], [8.0, 5.6], [9.0, 5.6], [10.0, 5.6], [11.0, 5.6]], "suspicious:blink": [[0.5, 2.0], [7.5, 2.0], [1.5, 2.0], [8.5, 2.0], [2.5, 2.0], [9.5, 2.0], [3.5, 2.0], [10.5, 2.0], [4.5, 2.0], [11.5, 2.0], [5.5, 2.0], [12.5, 2.0], [2.0, 5.6], [3.0, 5.6], [4.0, 5.6], [5.0, 5.6], [6.0, 5.6], [7.0, 5.6], [8.0, 5.6], [9.0, 5.6], [10.0, 5.6], [11.0, 5.6]], "success": [[0.5, 2.0], [7.5, 2.0], [1.5, 2.0], [8.5, 2.0], [4.5, 2.0], [11.5, 2.0], [5.5, 2.0], [12.5, 2.0], [1.5, 3.0], [8.5, 3.0], [2.5, 3.0], [9.5, 3.0], [3.5, 3.0], [10.5, 3.0], [4.5, 3.0], [11.5, 3.0], [2.5, 4.0], [9.5, 4.0], [3.5, 4.0], [10.5, 4.0], [2.0, 5.6], [3.0, 5.6], [4.0, 5.6], [5.0, 5.6], [6.0, 5.6], [7.0, 5.6], [8.0, 5.6], [9.0, 5.6], [10.0, 5.6], [11.0, 5.6], [3.0, 6.6], [5.0, 6.6], [7.0, 6.6], [9.0, 6.6]], "success:blink": [[0.5, 2.0], [7.5, 2.0], [1.5, 2.0], [8.5, 2.0], [2.5, 2.0], [9.5, 2.0], [3.5, 2.0], [10.5, 2.0], [4.5, 2.0], [11.5, 2.0], [5.5, 2.0], [12.5, 2.0], [2.0, 5.6], [3.0, 5.6], [4.0, 5.6], [5.0, 5.6], [6.0, 5.6], [7.0, 5.6], [8.0, 5.6], [9.0, 5.6], [10.0, 5.6], [11.0, 5.6], [3.0, 6.6], [5.0, 6.6], [7.0, 6.6], [9.0, 6.6]], "warning": [[2.5, 2.0], [9.5, 2.0], [3.5, 2.0], [10.5, 2.0], [2.5, 3.0], [9.5, 3.0], [3.5, 3.0], [10.5, 3.0], [2.5, 4.0], [9.5, 4.0], [3.5, 4.0], [10.5, 4.0], [2.0, 5.6], [3.0, 5.6], [4.0, 5.6], [5.0, 5.6], [6.0, 5.6], [7.0, 5.6], [8.0, 5.6], [9.0, 5.6], [10.0, 5.6], [11.0, 5.6]], "warning:blink": [[0.5, 2.0], [7.5, 2.0], [1.5, 2.0], [8.5, 2.0], [2.5, 2.0], [9.5, 2.0], [3.5, 2.0], [10.5, 2.0], [4.5, 2.0], [11.5, 2.0], [5.5, 2.0], [12.5, 2.0], [2.0, 5.6], [3.0, 5.6], [4.0, 5.6], [5.0, 5.6], [6.0, 5.6], [7.0, 5.6], [8.0, 5.6], [9.0, 5.6], [10.0, 5.6], [11.0, 5.6]], "error": [[0.5, 2.0], [7.5, 2.0], [1.5, 2.0], [8.5, 2.0], [4.5, 2.0], [11.5, 2.0], [5.5, 2.0], [12.5, 2.0], [1.5, 3.0], [8.5, 3.0], [2.5, 3.0], [9.5, 3.0], [3.5, 3.0], [10.5, 3.0], [4.5, 3.0], [11.5, 3.0], [0.5, 4.0], [7.5, 4.0], [1.5, 4.0], [8.5, 4.0], [4.5, 4.0], [11.5, 4.0], [5.5, 4.0], [12.5, 4.0], [2.0, 5.6], [3.0, 5.6], [4.0, 5.6], [5.0, 5.6], [6.0, 5.6], [7.0, 5.6], [8.0, 5.6], [9.0, 5.6], [10.0, 5.6], [11.0, 5.6], [3.0, 6.6], [5.0, 6.6], [7.0, 6.6], [9.0, 6.6]], "error:blink": [[0.5, 2.0], [7.5, 2.0], [1.5, 2.0], [8.5, 2.0], [2.5, 2.0], [9.5, 2.0], [3.5, 2.0], [10.5, 2.0], [4.5, 2.0], [11.5, 2.0], [5.5, 2.0], [12.5, 2.0], [2.0, 5.6], [3.0, 5.6], [4.0, 5.6], [5.0, 5.6], [6.0, 5.6], [7.0, 5.6], [8.0, 5.6], [9.0, 5.6], [10.0, 5.6], [11.0, 5.6], [3.0, 6.6], [5.0, 6.6], [7.0, 6.6], [9.0, 6.6]], "sleepy": [[0.5, 2.0], [7.5, 2.0], [1.5, 2.0], [8.5, 2.0], [2.5, 2.0], [9.5, 2.0], [3.5, 2.0], [10.5, 2.0], [4.5, 2.0], [11.5, 2.0], [5.5, 2.0], [12.5, 2.0], [4.0, 5.6], [5.0, 5.6], [6.0, 5.6], [7.0, 5.6], [8.0, 5.6], [9.0, 5.6]], "sleepy:blink": [[0.5, 2.0], [7.5, 2.0], [1.5, 2.0], [8.5, 2.0], [2.5, 2.0], [9.5, 2.0], [3.5, 2.0], [10.5, 2.0], [4.5, 2.0], [11.5, 2.0], [5.5, 2.0], [12.5, 2.0], [4.0, 5.6], [5.0, 5.6], [6.0, 5.6], [7.0, 5.6], [8.0, 5.6], [9.0, 5.6]], "building": [[1.5, 2.0], [8.5, 2.0], [4.5, 2.0], [11.5, 2.0], [0.5, 3.0], [7.5, 3.0], [1.5, 3.0], [8.5, 3.0], [2.5, 3.0], [9.5, 3.0], [3.5, 3.0], [10.5, 3.0], [4.5, 3.0], [11.5, 3.0], [5.5, 3.0], [12.5, 3.0], [1.5, 4.0], [8.5, 4.0], [4.5, 4.0], [11.5, 4.0], [2.0, 5.6], [3.0, 5.6], [4.0, 5.6], [5.0, 5.6], [6.0, 5.6], [7.0, 5.6], [8.0, 5.6], [9.0, 5.6], [10.0, 5.6], [11.0, 5.6], [3.0, 6.6], [5.0, 6.6], [7.0, 6.6], [9.0, 6.6]], "building:blink": [[0.5, 2.0], [7.5, 2.0], [1.5, 2.0], [8.5, 2.0], [2.5, 2.0], [9.5, 2.0], [3.5, 2.0], [10.5, 2.0], [4.5, 2.0], [11.5, 2.0], [5.5, 2.0], [12.5, 2.0], [2.0, 5.6], [3.0, 5.6], [4.0, 5.6], [5.0, 5.6], [6.0, 5.6], [7.0, 5.6], [8.0, 5.6], [9.0, 5.6], [10.0, 5.6], [11.0, 5.6], [3.0, 6.6], [5.0, 6.6], [7.0, 6.6], [9.0, 6.6]], "deploying": [[0.5, 2.0], [7.5, 2.0], [1.5, 2.0], [8.5, 2.0], [1.5, 3.0], [8.5, 3.0], [2.5, 3.0], [9.5, 3.0], [3.5, 3.0], [10.5, 3.0], [4.5, 3.0], [11.5, 3.0], [0.5, 4.0], [7.5, 4.0], [1.5, 4.0], [8.5, 4.0], [2.0, 5.6], [3.0, 5.6], [4.0, 5.6], [5.0, 5.6], [6.0, 5.6], [7.0, 5.6], [8.0, 5.6], [9.0, 5.6], [10.0, 5.6], [11.0, 5.6], [3.0, 6.6], [5.0, 6.6], [7.0, 6.6], [9.0, 6.6]], "deploying:blink": [[0.5, 2.0], [7.5, 2.0], [1.5, 2.0], [8.5, 2.0], [2.5, 2.0], [9.5, 2.0], [3.5, 2.0], [10.5, 2.0], [4.5, 2.0], [11.5, 2.0], [5.5, 2.0], [12.5, 2.0], [2.0, 5.6], [3.0, 5.6], [4.0, 5.6], [5.0, 5.6], [6.0, 5.6], [7.0, 5.6], [8.0, 5.6], [9.0, 5.6], [10.0, 5.6], [11.0, 5.6], [3.0, 6.6], [5.0, 6.6], [7.0, 6.6], [9.0, 6.6]], "brainstorm": [[1.5, 1.0], [8.5, 1.0], [2.5, 1.0], [9.5, 1.0], [3.5, 1.0], [10.5, 1.0], [4.5, 1.0], [11.5, 1.0], [0.5, 2.0], [7.5, 2.0], [1.5, 2.0], [8.5, 2.0], [4.5, 2.0], [11.5, 2.0], [5.5, 2.0], [12.5, 2.0], [0.5, 3.0], [7.5, 3.0], [1.5, 3.0], [8.5, 3.0], [3.5, 3.0], [10.5, 3.0], [4.5, 3.0], [11.5, 3.0], [5.5, 3.0], [12.5, 3.0], [2.5, 4.0], [9.5, 4.0], [3.5, 4.0], [10.5, 4.0], [2.0, 5.6], [3.0, 5.6], [4.0, 5.6], [5.0, 5.6], [6.0, 5.6], [7.0, 5.6], [8.0, 5.6], [9.0, 5.6], [10.0, 5.6], [11.0, 5.6], [3.0, 6.6], [5.0, 6.6], [7.0, 6.6], [9.0, 6.6]], "brainstorm:blink": [[0.5, 2.0], [7.5, 2.0], [1.5, 2.0], [8.5, 2.0], [2.5, 2.0], [9.5, 2.0], [3.5, 2.0], [10.5, 2.0], [4.5, 2.0], [11.5, 2.0], [5.5, 2.0], [12.5, 2.0], [2.0, 5.6], [3.0, 5.6], [4.0, 5.6], [5.0, 5.6], [6.0, 5.6], [7.0, 5.6], [8.0, 5.6], [9.0, 5.6], [10.0, 5.6], [11.0, 5.6], [3.0, 6.6], [5.0, 6.6], [7.0, 6.6], [9.0, 6.6]], "stealth": [[0.5, 2.0], [7.5, 2.0], [1.5, 2.0], [8.5, 2.0], [2.5, 2.0], [9.5, 2.0], [3.5, 2.0], [10.5, 2.0], [4.5, 2.0], [11.5, 2.0], [5.5, 2.0], [12.5, 2.0], [0.5, 3.0], [7.5, 3.0], [1.5, 3.0], [8.5, 3.0], [2.5, 3.0], [9.5, 3.0], [3.5, 3.0], [10.5, 3.0], [4.5, 3.0], [11.5, 3.0], [5.5, 3.0], [12.5, 3.0], [2.0, 5.6], [3.0, 5.6], [4.0, 5.6], [5.0, 5.6], [6.0, 5.6], [7.0, 5.6], [8.0, 5.6], [9.0, 5.6], [10.0, 5.6], [11.0, 5.6]], "stealth:blink": [[0.5, 2.0], [7.5, 2.0], [1.5, 2.0], [8.5, 2.0], [2.5, 2.0], [9.5, 2.0], [3.5, 2.0], [10.5, 2.0], [4.5, 2.0], [11.5, 2.0], [5.5, 2.0], [12.5, 2.0], [2.0, 5.6], [3.0, 5.6], [4.0, 5.6], [5.0, 5.6], [6.0, 5.6], [7.0, 5.6], [8.0, 5.6], [9.0, 5.6], [10.0, 5.6], [11.0, 5.6]], "broken": [[0.5, 2.0], [7.5, 2.0], [1.5, 2.0], [8.5, 2.0], [4.5, 2.0], [11.5, 2.0], [5.5, 2.0], [12.5, 2.0], [1.5, 3.0], [8.5, 3.0], [2.5, 3.0], [9.5, 3.0], [3.5, 3.0], [10.5, 3.0], [4.5, 3.0], [11.5, 3.0], [0.5, 4.0], [7.5, 4.0], [1.5, 4.0], [8.5, 4.0], [4.5, 4.0], [11.5, 4.0], [5.5, 4.0], [12.5, 4.0], [2.0, 5.6], [3.0, 5.6], [6.0, 5.6], [7.0, 5.6], [10.0, 5.6], [11.0, 5.6], [5.0, 6.6], [9.0, 6.6]], "broken:blink": [[0.5, 2.0], [7.5, 2.0], [1.5, 2.0], [8.5, 2.0], [2.5, 2.0], [9.5, 2.0], [3.5, 2.0], [10.5, 2.0], [4.5, 2.0], [11.5, 2.0], [5.5, 2.0], [12.5, 2.0], [2.0, 5.6], [3.0, 5.6], [6.0, 5.6], [7.0, 5.6], [10.0, 5.6], [11.0, 5.6], [5.0, 6.6], [9.0, 6.6]], "default:blink:blink": [[0.5, 2.0], [7.5, 2.0], [1.5, 2.0], [8.5, 2.0], [2.5, 2.0], [9.5, 2.0], [3.5, 2.0], [10.5, 2.0], [4.5, 2.0], [11.5, 2.0], [5.5, 2.0], [12.5, 2.0], [2.0, 5.6], [3.0, 5.6], [4.0, 5.6], [5.0, 5.6], [6.0, 5.6], [7.0, 5.6], [8.0, 5.6], [9.0, 5.6], [10.0, 5.6], [11.0, 5.6], [3.0, 6.6], [5.0, 6.6], [7.0, 6.6], [9.0, 6.6]], "happy:blink:blink": [[0.5, 2.0], [7.5, 2.0], [1.5, 2.0], [8.5, 2.0], [2.5, 2.0], [9.5, 2.0], [3.5, 2.0], [10.5, 2.0], [4.5, 2.0], [11.5, 2.0], [5.5, 2.0], [12.5, 2.0], [2.0, 5.6], [3.0, 5.6], [4.0, 5.6], [5.0, 5.6], [6.0, 5.6], [7.0, 5.6], [8.0, 5.6], [9.0, 5.6], [10.0, 5.6], [11.0, 5.6], [3.0, 6.6], [5.0, 6.6], [7.0, 6.6], [9.0, 6.6]], "excited:blink:blink": [[0.5, 2.0], [7.5, 2.0], [1.5, 2.0], [8.5, 2.0], [2.5, 2.0], [9.5, 2.0], [3.5, 2.0], [10.5, 2.0], [4.5, 2.0], [11.5, 2.0], [5.5, 2.0], [12.5, 2.0], [2.0, 5.6], [3.0, 5.6], [4.0, 5.6], [5.0, 5.6], [6.0, 5.6], [7.0, 5.6], [8.0, 5.6], [9.0, 5.6], [10.0, 5.6], [11.0, 5.6], [3.0, 6.6], [5.0, 6.6], [7.0, 6.6], [9.0, 6.6]], "focused:blink:blink": [[0.5, 2.0], [7.5, 2.0], [1.5, 2.0], [8.5, 2.0], [2.5, 2.0], [9.5, 2.0], [3.5, 2.0], [10.5, 2.0], [4.5, 2.0], [11.5, 2.0], [5.5, 2.0], [12.5, 2.0], [2.0, 5.6], [3.0, 5.6], [4.0, 5.6], [5.0, 5.6], [6.0, 5.6], [7.0, 5.6], [8.0, 5.6], [9.0, 5.6], [10.0, 5.6], [11.0, 5.6]], "thinking:blink:blink": [[0.5, 2.0], [7.5, 2.0], [1.5, 2.0], [8.5, 2.0], [2.5, 2.0], [9.5, 2.0], [3.5, 2.0], [10.5, 2.0], [4.5, 2.0], [11.5, 2.0], [5.5, 2.0], [12.5, 2.0], [4.0, 5.6], [5.0, 5.6], [6.0, 5.6], [7.0, 5.6], [8.0, 5.6], [9.0, 5.6]], "analyzing:blink:blink": [[0.5, 2.0], [7.5, 2.0], [1.5, 2.0], [8.5, 2.0], [2.5, 2.0], [9.5, 2.0], [3.5, 2.0], [10.5, 2.0], [4.5, 2.0], [11.5, 2.0], [5.5, 2.0], [12.5, 2.0], [2.0, 5.6], [3.0, 5.6], [4.0, 5.6], [5.0, 5.6], [6.0, 5.6], [7.0, 5.6], [8.0, 5.6], [9.0, 5.6], [10.0, 5.6], [11.0, 5.6], [3.0, 6.6], [5.0, 6.6], [7.0, 6.6], [9.0, 6.6]], "suspicious:blink:blink": [[0.5, 2.0], [7.5, 2.0], [1.5, 2.0], [8.5, 2.0], [2.5, 2.0], [9.5, 2.0], [3.5, 2.0], [10.5, 2.0], [4.5, 2.0], [11.5, 2.0], [5.5, 2.0], [12.5, 2.0], [2.0, 5.6], [3.0, 5.6], [4.0, 5.6], [5.0, 5.6], [6.0, 5.6], [7.0, 5.6], [8.0, 5.6], [9.0, 5.6], [10.0, 5.6], [11.0, 5.6]], "success:blink:blink": [[0.5, 2.0], [7.5, 2.0], [1.5, 2.0], [8.5, 2.0], [2.5, 2.0], [9.5, 2.0], [3.5, 2.0], [10.5, 2.0], [4.5, 2.0], [11.5, 2.0], [5.5, 2.0], [12.5, 2.0], [2.0, 5.6], [3.0, 5.6], [4.0, 5.6], [5.0, 5.6], [6.0, 5.6], [7.0, 5.6], [8.0, 5.6], [9.0, 5.6], [10.0, 5.6], [11.0, 5.6], [3.0, 6.6], [5.0, 6.6], [7.0, 6.6], [9.0, 6.6]], "warning:blink:blink": [[0.5, 2.0], [7.5, 2.0], [1.5, 2.0], [8.5, 2.0], [2.5, 2.0], [9.5, 2.0], [3.5, 2.0], [10.5, 2.0], [4.5, 2.0], [11.5, 2.0], [5.5, 2.0], [12.5, 2.0], [2.0, 5.6], [3.0, 5.6], [4.0, 5.6], [5.0, 5.6], [6.0, 5.6], [7.0, 5.6], [8.0, 5.6], [9.0, 5.6], [10.0, 5.6], [11.0, 5.6]], "error:blink:blink": [[0.5, 2.0], [7.5, 2.0], [1.5, 2.0], [8.5, 2.0], [2.5, 2.0], [9.5, 2.0], [3.5, 2.0], [10.5, 2.0], [4.5, 2.0], [11.5, 2.0], [5.5, 2.0], [12.5, 2.0], [2.0, 5.6], [3.0, 5.6], [4.0, 5.6], [5.0, 5.6], [6.0, 5.6], [7.0, 5.6], [8.0, 5.6], [9.0, 5.6], [10.0, 5.6], [11.0, 5.6], [3.0, 6.6], [5.0, 6.6], [7.0, 6.6], [9.0, 6.6]], "sleepy:blink:blink": [[0.5, 2.0], [7.5, 2.0], [1.5, 2.0], [8.5, 2.0], [2.5, 2.0], [9.5, 2.0], [3.5, 2.0], [10.5, 2.0], [4.5, 2.0], [11.5, 2.0], [5.5, 2.0], [12.5, 2.0], [4.0, 5.6], [5.0, 5.6], [6.0, 5.6], [7.0, 5.6], [8.0, 5.6], [9.0, 5.6]], "building:blink:blink": [[0.5, 2.0], [7.5, 2.0], [1.5, 2.0], [8.5, 2.0], [2.5, 2.0], [9.5, 2.0], [3.5, 2.0], [10.5, 2.0], [4.5, 2.0], [11.5, 2.0], [5.5, 2.0], [12.5, 2.0], [2.0, 5.6], [3.0, 5.6], [4.0, 5.6], [5.0, 5.6], [6.0, 5.6], [7.0, 5.6], [8.0, 5.6], [9.0, 5.6], [10.0, 5.6], [11.0, 5.6], [3.0, 6.6], [5.0, 6.6], [7.0, 6.6], [9.0, 6.6]], "deploying:blink:blink": [[0.5, 2.0], [7.5, 2.0], [1.5, 2.0], [8.5, 2.0], [2.5, 2.0], [9.5, 2.0], [3.5, 2.0], [10.5, 2.0], [4.5, 2.0], [11.5, 2.0], [5.5, 2.0], [12.5, 2.0], [2.0, 5.6], [3.0, 5.6], [4.0, 5.6], [5.0, 5.6], [6.0, 5.6], [7.0, 5.6], [8.0, 5.6], [9.0, 5.6], [10.0, 5.6], [11.0, 5.6], [3.0, 6.6], [5.0, 6.6], [7.0, 6.6], [9.0, 6.6]], "brainstorm:blink:blink": [[0.5, 2.0], [7.5, 2.0], [1.5, 2.0], [8.5, 2.0], [2.5, 2.0], [9.5, 2.0], [3.5, 2.0], [10.5, 2.0], [4.5, 2.0], [11.5, 2.0], [5.5, 2.0], [12.5, 2.0], [2.0, 5.6], [3.0, 5.6], [4.0, 5.6], [5.0, 5.6], [6.0, 5.6], [7.0, 5.6], [8.0, 5.6], [9.0, 5.6], [10.0, 5.6], [11.0, 5.6], [3.0, 6.6], [5.0, 6.6], [7.0, 6.6], [9.0, 6.6]], "stealth:blink:blink": [[0.5, 2.0], [7.5, 2.0], [1.5, 2.0], [8.5, 2.0], [2.5, 2.0], [9.5, 2.0], [3.5, 2.0], [10.5, 2.0], [4.5, 2.0], [11.5, 2.0], [5.5, 2.0], [12.5, 2.0], [2.0, 5.6], [3.0, 5.6], [4.0, 5.6], [5.0, 5.6], [6.0, 5.6], [7.0, 5.6], [8.0, 5.6], [9.0, 5.6], [10.0, 5.6], [11.0, 5.6]], "broken:blink:blink": [[0.5, 2.0], [7.5, 2.0], [1.5, 2.0], [8.5, 2.0], [2.5, 2.0], [9.5, 2.0], [3.5, 2.0], [10.5, 2.0], [4.5, 2.0], [11.5, 2.0], [5.5, 2.0], [12.5, 2.0], [2.0, 5.6], [3.0, 5.6], [6.0, 5.6], [7.0, 5.6], [10.0, 5.6], [11.0, 5.6], [5.0, 6.6], [9.0, 6.6]]};
  var COLORS = {"default": "#2EC5FF", "happy": "#3CFF7A", "excited": "#F5EF34", "focused": "#2E86FF", "thinking": "#FF60FF", "analyzing": "#C259FF", "suspicious": "#FF985A", "success": "#00FFB2", "warning": "#F5EF34", "error": "#FF4757", "sleepy": "#858392", "building": "#4FBEFE", "deploying": "#00C2B2", "brainstorm": "#FF79D0", "stealth": "#605F6B", "broken": "#EB4268", "default:blink": "#2EC5FF", "happy:blink": "#3CFF7A", "excited:blink": "#F5EF34", "focused:blink": "#2E86FF", "thinking:blink": "#FF60FF", "analyzing:blink": "#C259FF", "suspicious:blink": "#FF985A", "success:blink": "#00FFB2", "warning:blink": "#F5EF34", "error:blink": "#FF4757", "sleepy:blink": "#858392", "building:blink": "#4FBEFE", "deploying:blink": "#00C2B2", "brainstorm:blink": "#FF79D0", "stealth:blink": "#605F6B", "broken:blink": "#EB4268"};
  var HUES = {"default": 0, "happy": -58, "excited": -139, "focused": 18, "thinking": 103, "analyzing": 81, "suspicious": -174, "success": -35, "warning": -139, "error": 158, "sleepy": 51, "building": 5, "deploying": -22, "brainstorm": 124, "stealth": 48, "broken": 150};
  var FILTERS = {};
  var SATS = {"sleepy": 0.25, "stealth": 0.22};
  var CAPTIONS = {"default": "Ready. Always.", "happy": "Good stuff!", "excited": "Let's go!", "focused": "Deep work...", "thinking": "Processing...", "analyzing": "Running the numbers.", "suspicious": "Hmmmm...", "success": "Nailed it.", "warning": "Careful...", "error": "Nope.", "sleepy": "Later...", "building": "Compiling...", "deploying": "Shipping...", "brainstorm": "Ideas incoming!", "stealth": "Watching...", "broken": "Fix me...", "default:blink": "Ready. Always.", "happy:blink": "Good stuff!", "excited:blink": "Let's go!", "focused:blink": "Deep work...", "thinking:blink": "Processing...", "analyzing:blink": "Running the numbers.", "suspicious:blink": "Hmmmm...", "success:blink": "Nailed it.", "warning:blink": "Careful...", "error:blink": "Nope.", "sleepy:blink": "Later...", "building:blink": "Compiling...", "deploying:blink": "Shipping...", "brainstorm:blink": "Ideas incoming!", "stealth:blink": "Watching...", "broken:blink": "Fix me..."};

  var MOOD = 'default', pending = null, pendingUntil = 0;
  var startedAt = Date.now();
  var root, faceEl, blinkTimer, idleTimer, moodTimer, scanQueued = false;

  /* Each visible founder has its own resting pose and one-shot gestures.
     Six CSS inches is an edge-to-edge 576px neighborhood at the current zoom. */
  var FACE_SELECTOR = '.ch-logo, .brand-logo, .wi-face, .ws-mascot';
  var FACE_POSES = [
    { name: 'relaxed', row: 0, frame: 0, weight: 5 },
    { name: 'smile', row: 0, frame: 5, weight: 4 },
    { name: 'glance-up', row: 2, frame: 5, weight: 2 },
    { name: 'glance-left', row: 5, frame: 3, weight: 2 },
    { name: 'glance-right', row: 5, frame: 8, weight: 2 },
    { name: 'thoughtful', row: 4, frame: 4, weight: 1 }
  ];
  var faceMedia = window.matchMedia('(prefers-reduced-motion: reduce)');
  var founderFaces = [], founderFrame = null, founderWake = null, founderScanQueued = false;
  var lastFounderGesture = -Infinity;
  var founderGeometryAt = 0, composerFounder = null, founderPointer = { active: false, x: 0, y: 0 };
  var LOOK_YAW = [[5, 3], [5, 2], [5, 1], [0, 0], [5, 9], [5, 8], [5, 6]];
  var LOOK_PITCH = [[4, 2], [4, 1], [0, 0], [4, 3], [4, 4], [4, 5]];

  function faceRandom(min, max) { return min + Math.random() * (max - min); }

  function faceVisible(el) {
    var box = el.getBoundingClientRect(), style = getComputedStyle(el);
    return el.isConnected && box.width > 0 && box.height > 0 && box.bottom > 0 && box.right > 0 &&
      box.top < innerHeight && box.left < innerWidth && style.visibility !== 'hidden' && style.display !== 'none';
  }

  function facesNear(a, b) {
    return faceDistance(a.box, b.box) < 576;
  }

  function faceDistance(x, y) {
    if (!x || !y) return Infinity;
    return Math.hypot(Math.max(0, x.left - y.right, y.left - x.right), Math.max(0, x.top - y.bottom, y.top - x.bottom));
  }

  function chooseComposerFounder() {
    var composers = Array.from(document.querySelectorAll('.composer-card')).filter(faceVisible).map(function (el) { return el.getBoundingClientRect(); });
    var candidate = founderFaces.find(function (state) { return state.visible && state.el.classList.contains('ws-mascot'); });
    if (!candidate && composers.length) {
      var nearest = 256;
      founderFaces.forEach(function (state) {
        if (!state.visible || !state.el.classList.contains('wi-face')) return;
        var distance = Math.min.apply(null, composers.map(function (box) { return faceDistance(state.box, box); }));
        if (distance < nearest) { candidate = state; nearest = distance; }
      });
    }
    if (composerFounder !== candidate) {
      if (composerFounder) {
        composerFounder.el.classList.remove('mega-founder-follows');
        composerFounder.el.style.removeProperty('--mega-founder-lean');
        composerFounder.look = null;
      }
      composerFounder = candidate || null;
    }
    if (composerFounder) {
      composerFounder.el.classList.add('mega-founder-follows');
      if (founderPointer.active && !faceMedia.matches) beginFounderLook();
    }
  }

  function beginFounderLook() {
    if (!composerFounder || !composerFounder.visible) return;
    composerFounder.motion = null;
    if (!composerFounder.look) composerFounder.look = { x: 0, y: 0, at: performance.now(), axis: 'x' };
  }

  function moveFounderPointer(event) {
    if (event.pointerType === 'touch' || faceMedia.matches || document.hidden) return;
    founderPointer = { active: true, x: event.clientX, y: event.clientY };
    beginFounderLook();
    if (composerFounder && founderFrame === null) wakeFounders();
  }

  function releaseFounderPointer() {
    founderPointer.active = false;
    if (composerFounder && composerFounder.look) wakeFounders();
  }

  function tickFounderLook(state, now) {
    var look = state.look, box = state.box;
    var targetX = founderPointer.active ? Math.max(-1, Math.min(1, (founderPointer.x - box.left - box.width / 2) / 180)) : 0;
    var targetY = founderPointer.active ? Math.max(-1, Math.min(1, (founderPointer.y - box.top - box.height / 2) / 160)) : 0;
    var alpha = 1 - Math.exp(-Math.min(50, Math.max(0, now - look.at)) / 120);
    look.at = now; look.x += (targetX - look.x) * alpha; look.y += (targetY - look.y) * alpha;
    var moving = Math.abs(targetX - look.x) + Math.abs(targetY - look.y) > .006;
    if (!moving) { look.x = targetX; look.y = targetY; }
    if (Math.abs(look.y) > Math.abs(look.x) * 1.2) look.axis = 'y';
    else if (Math.abs(look.x) > Math.abs(look.y) * 1.2) look.axis = 'x';
    var cell = look.axis === 'x' ? LOOK_YAW[Math.round(look.x * 3) + 3] : LOOK_PITCH[look.y < 0 ? 2 + Math.round(look.y * 2) : 2 + Math.round(look.y * 3)];
    state.pose = { name: 'follow-' + (look.axis === 'x' ? look.x < -.16 ? 'left' : look.x > .16 ? 'right' : 'center' : look.y < -.16 ? 'up' : look.y > .16 ? 'down' : 'center'), row: cell[0], frame: cell[1] };
    /* Reserve the cursor's pose before reallocating a nearby held expression. */
    state.row = cell[0]; state.frame = cell[1];
    founderFaces.forEach(function (other) {
      if (other !== state && other.visible && facesNear(state, other) && faceCellKey(other.row, other.frame) === faceCellKey(cell[0], cell[1])) settleFounder(other, chooseFounderPose(other), now);
    });
    paintFounder(state, cell[0], cell[1]);
    state.el.style.setProperty('--mega-founder-lean', 'translate(' + (look.x * 1.5).toFixed(3) + 'px,' + (look.y * 1.2).toFixed(3) + 'px) rotate(' + (look.x * 2).toFixed(3) + 'deg)');
    if (!founderPointer.active && !moving) {
      state.look = null;
      state.el.style.removeProperty('--mega-founder-lean');
      settleFounder(state, FACE_POSES[0], now);
    }
    look.moving = moving;
    return moving;
  }

  function faceCellKey(row, frame) {
    return frame === 0 || frame === 11 ? 'rest' : row + ':' + frame;
  }

  function occupiedFace(state, row, frame) {
    var key = faceCellKey(row, frame);
    return founderFaces.some(function (other) {
      return other !== state && other.visible && facesNear(state, other) &&
        faceCellKey(other.row, other.frame) === key;
    });
  }

  function paintFounder(state, row, frame) {
    if (occupiedFace(state, row, frame)) return;
    state.row = row; state.frame = frame;
    state.el.style.setProperty('--floyd-face-x', (frame / 11 * 100) + '%');
    state.el.style.setProperty('--floyd-face-y', (row / 5 * 100) + '%');
    state.el.dataset.megafloydExpression = state.pose.name;
  }

  function chooseFounderPose(state) {
    var neighbors = founderFaces.filter(function (other) { return other !== state && other.visible && other.pose && facesNear(state, other); });
    var available = FACE_POSES.filter(function (pose) {
      return !neighbors.some(function (other) { return other.pose.name === pose.name; }) && !occupiedFace(state, pose.row, pose.frame);
    });
    var distinct = available.filter(function (pose) { return !founderFaces.some(function (other) { return other !== state && other.visible && other.pose && other.pose.name === pose.name; }); });
    var pool = distinct.length ? distinct : available;
    var fresh = pool.filter(function (pose) { return state.history.indexOf(pose.name) === -1; });
    var choices = fresh.length ? fresh : pool;
    if (!choices.length) {
      /* Extra simultaneously visible heads can use another existing, distinct
         held cell instead of disappearing or waiting for a free pose forever. */
      for (var row = 0; row < 6; row++) for (var frame = 2; frame < 10; frame++) {
        if (!occupiedFace(state, row, frame)) return { name: 'detail-' + row + '-' + frame, row: row, frame: frame, weight: 1 };
      }
      return state.pose || FACE_POSES[0];
    }
    var total = choices.reduce(function (sum, pose) { return sum + pose.weight; }, 0), pick = Math.random() * total;
    for (var i = 0; i < choices.length; i++) { pick -= choices[i].weight; if (pick <= 0) return choices[i]; }
    return choices[choices.length - 1];
  }

  function settleFounder(state, pose, now) {
    state.pose = pose; state.motion = null;
    state.history = state.history.concat(pose.name).slice(-2);
    paintFounder(state, pose.row, pose.frame);
    state.nextAt = now + faceRandom(4200, 10800) * state.tempo;
  }

  function startFounderGesture(state, now) {
    var previous = state.pose;
    var next = now - state.changedAt > 14000 && Math.random() < .35 ? chooseFounderPose(state) : previous;
    var cells = [];
    function add(row, from, to) {
      var direction = from <= to ? 1 : -1;
      for (var frame = from; direction > 0 ? frame <= to : frame >= to; frame += direction) cells.push({ row: row, frame: frame });
    }
    if (next.name !== previous.name) {
      add(state.row, state.frame, 11);
      add(next.row, 0, next.frame);
      state.changedAt = now;
      state.history = state.history.concat(next.name).slice(-2);
    } else if (previous.name === 'relaxed') {
      add(3, 1, 6); add(3, 5, 1); cells.push({ row: 0, frame: 0 });
    } else {
      var excursion = Math.min(9, Math.max(2, previous.frame + (Math.random() < .5 ? -2 : 2)));
      add(previous.row, previous.frame, excursion); add(previous.row, excursion, previous.frame);
    }
    state.pose = next;
    var elapsed = 0;
    cells.forEach(function (cell, index) {
      var progress = cells.length > 1 ? index / (cells.length - 1) : 1;
      var duration = previous.name === 'relaxed' && next === previous ? 23 + 12 * Math.abs(progress * 2 - 1) : 52 + 55 * Math.abs(progress * 2 - 1);
      if (index === Math.floor(cells.length / 2)) duration += next === previous && previous.name === 'relaxed' ? 35 : faceRandom(100, 230);
      elapsed += duration * state.tempo;
      cell.until = elapsed;
    });
    state.motion = { startedAt: now, cells: cells, duration: elapsed };
    lastFounderGesture = now;
  }

  function stopFounderClock() {
    if (founderFrame !== null) cancelAnimationFrame(founderFrame);
    clearTimeout(founderWake); founderFrame = null; founderWake = null;
  }

  function tickFounders(now) {
    founderFrame = null;
    if (document.hidden || faceMedia.matches) return;
    var active = false, nextAt = Infinity;
    if (composerFounder && composerFounder.look && now - founderGeometryAt > 250) {
      founderFaces.forEach(function (state) { state.box = state.el.getBoundingClientRect(); });
      founderGeometryAt = now;
    }
    founderFaces.forEach(function (state) {
      if (!state.visible) return;
      if (state.look) { active = tickFounderLook(state, now) || active; if (state.look) return; }
      if (!state.motion && now >= state.nextAt) {
        var nearbyMoving = founderFaces.some(function (other) { return other !== state && other.visible && (other.motion || (other.look && other.look.moving)) && facesNear(state, other); });
        if (nearbyMoving || now - lastFounderGesture < 450) state.nextAt = now + faceRandom(650, 1600);
        else startFounderGesture(state, now);
      }
      if (state.motion) {
        var elapsed = now - state.motion.startedAt;
        var cell = state.motion.cells.find(function (part) { return elapsed < part.until; });
        if (cell) { paintFounder(state, cell.row, cell.frame); active = true; }
        else settleFounder(state, state.pose, now);
      }
      nextAt = Math.min(nextAt, state.nextAt);
    });
    if (active) founderFrame = requestAnimationFrame(tickFounders);
    else if (nextAt < Infinity) founderWake = setTimeout(function () { founderWake = null; founderFrame = requestAnimationFrame(tickFounders); }, Math.max(30, nextAt - performance.now()));
  }

  function scanFounderHeads() {
    founderScanQueued = false;
    var now = performance.now();
    founderFaces = founderFaces.filter(function (state) { return state.el.isConnected; });
    document.querySelectorAll(FACE_SELECTOR).forEach(function (el) {
      if (el.classList.contains('brand-logo')) {
        var size = parseFloat(el.style.width) || 64;
        el.style.setProperty('--mega-founder-size', size * 2 + 'px');
      }
      if (!founderFaces.some(function (state) { return state.el === el; })) {
        var state = { el: el, pose: null, row: -1, frame: -1, visible: false, history: [], tempo: faceRandom(.86, 1.18), nextAt: now, changedAt: now, motion: null };
        founderFaces.push(state);
        el.addEventListener('click', function () {
          if (!faceMedia.matches && !document.hidden) { state.nextAt = performance.now() + faceRandom(80, 450); wakeFounders(); }
        });
      }
    });
    founderFaces.forEach(function (state) { state.box = state.el.getBoundingClientRect(); state.visible = faceVisible(state.el); });
    founderGeometryAt = now;
    chooseComposerFounder();
    founderFaces.forEach(function (state) {
      if (!state.visible) { state.motion = null; state.look = null; return; }
      if (state.look) return;
      if (!state.pose || occupiedFace(state, state.row, state.frame) || founderFaces.some(function (other) { return other !== state && other.visible && other.pose && state.pose && other.pose.name === state.pose.name && facesNear(state, other); })) {
        settleFounder(state, chooseFounderPose(state), now);
        state.nextAt = now + faceRandom(1100, 4300) * state.tempo;
      }
    });
    wakeFounders();
  }

  function queueFounderScan() {
    if (founderScanQueued) return;
    founderScanQueued = true;
    requestAnimationFrame(scanFounderHeads);
  }

  function wakeFounders() {
    stopFounderClock();
    if (!document.hidden && !faceMedia.matches) founderFrame = requestAnimationFrame(tickFounders);
  }

  function resetFounderMotion() {
    stopFounderClock();
    var now = performance.now();
    founderPointer.active = false;
    founderFaces.forEach(function (state) {
      state.look = null; state.el.style.removeProperty('--mega-founder-lean');
      if (state.pose) settleFounder(state, state.pose, now);
      state.nextAt = now + faceRandom(1200, 4800) * state.tempo;
    });
    if (!document.hidden) queueFounderScan();
  }

  function startFounders() {
    faceMedia.addEventListener('change', resetFounderMotion);
    document.addEventListener('visibilitychange', resetFounderMotion);
    window.addEventListener('pointermove', moveFounderPointer, { passive: true });
    window.addEventListener('pointerout', function (event) { if (!event.relatedTarget) releaseFounderPointer(); }, { passive: true });
    window.addEventListener('blur', releaseFounderPointer);
    document.addEventListener('load', queueFounderScan, true);
    if (document.fonts) document.fonts.ready.then(queueFounderScan);
    window.addEventListener('resize', queueFounderScan, { passive: true });
    window.addEventListener('scroll', queueFounderScan, { passive: true, capture: true });
    window.addEventListener('pagehide', function () { releaseFounderPointer(); stopFounderClock(); });
    window.addEventListener('pageshow', resetFounderMotion);
    scanFounderHeads();
  }

  function paint(mood) {
    if (!root || !faceEl) return;
    var base = mood.replace(':blink', '');
    if (base === 'stealth') base = 'sleepy';
    faceEl.style.filter = FILTERS[base] || '';
    var src = '/expressions/' + base + (mood.indexOf(':blink') !== -1 ? '-blink' : '') + '.webp';
    if (faceEl.getAttribute('src') !== src) faceEl.setAttribute('src', src);
    root.classList.toggle('fm-think', base === 'thinking' || base === 'building' || base === 'analyzing');
    root.title = CAPTIONS[base] || '';
  }

  function setMood(mood, holdMs) {
    if (!CELLS[mood]) mood = 'default';
    MOOD = mood;
    root.classList.toggle('fm-think', mood === 'thinking' || mood === 'building' || mood === 'analyzing');
    paint(mood);
    if (holdMs) {
      pending = mood;
      pendingUntil = Date.now() + holdMs;
      clearTimeout(moodTimer);
      moodTimer = setTimeout(function () {
        if (Date.now() >= pendingUntil) { pending = null; settle(); }
      }, holdMs);
    }
  }

  function blink(hold) {
    if (!pending && MOOD.replace(':blink','') !== 'success') {
      var m = MOOD;
      paint(m + ':blink');
      setTimeout(function () { if (MOOD === m && !pending) paint(m); }, hold || 110);
    }
    clearTimeout(blinkTimer);
    blinkTimer = setTimeout(blink, 2400 + Math.random() * 4200);
  }

  function queueScan() {
    queueFounderScan();
    if (scanQueued) return;
    scanQueued = true;
    setTimeout(function () { scanQueued = false; scan(); }, 350);
  }

  function scan() {
    if (pending) return;
    var q = document.querySelector.bind(document);
    var busy = q('[aria-busy="true"], [class*="spinner"], [class*="pending"], [class*="thinking"], [class*="streaming"]');
    var err = q('[class*="toast-error"], [class*="error-banner"], [role="alert"][data-variant="error"]');
    if (err) return setMood('error', 2600);
    if (busy) return setMood('thinking');
    var tail = (document.body.innerText || '').slice(-1200);
    if (/deploy|shipping|publishing/i.test(tail)) return setMood('deploying', 2200);
    if (/building|compiling|installing/i.test(tail)) return setMood('building', 2200);
    settle();
  }

  function settle() {
    if (pending) return;
    if (Date.now() - startedAt < 9000) return setMood('default');
    var opts = ['default', 'default', 'focused', 'happy', 'suspicious'];
    setMood(opts[Math.floor(Math.random() * opts.length)]);
  }

  function idle() {
    if (!pending && Date.now() > pendingUntil) {
      var opts = ['default', 'happy', 'sleepy', 'focused', 'brainstorm'];
      setMood(opts[Math.floor(Math.random() * opts.length)]);
    }
    idleTimer = setTimeout(idle, 11000 + Math.random() * 9000);
  }

  function place() {
    if (!root) return;
    var w = root.offsetWidth || 150;
    var anchor = document.querySelector('.composer-card') || document.querySelector('.chat-dock');
    var left = 16, bottom = 16;
    if (anchor) {
      var r = anchor.getBoundingClientRect();
      left = r.left - w - 18;
      bottom = Math.max(12, window.innerHeight - r.bottom - 2);
    }
    if (left < 10) left = 10;
    root.style.transform = 'translate3d(' + Math.round(left) + 'px,0,0)';
    root.style.top = 'auto';
    root.style.bottom = Math.round(bottom) + 'px';
  }

  function start() {
    root = document.createElement('div');
    root.id = 'floyd-mascot';
    root.setAttribute('aria-hidden', 'true');
    root.innerHTML =
      '<img class="fm-cabinet" src="/mascot-cabinet.webp" alt="" draggable="false">' +
      '<div class="fm-screen"><img alt="" draggable="false"></div>';
    document.body.appendChild(root);
    faceEl = root.querySelector('.fm-screen img');

    paint('default');
    place();
    requestAnimationFrame(function () { root.classList.add('fm-in'); });

    var cab = root.querySelector('.fm-cabinet');
    cab.addEventListener('click', function () { setMood('excited', 1800); });

    window.addEventListener('resize', place, { passive: true });
    window.addEventListener('load', place);
    new MutationObserver(queueScan).observe(document.body, { childList: true, subtree: true, characterData: true });
    setInterval(place, 1500);
    setTimeout(function () { place(); scan(); }, 350);
    preload();
    startFounders();
    blinkTimer = setTimeout(blink, 2200);
    idleTimer = setTimeout(idle, 12000);
  }

  function preload() {
    var names = ['default','happy','excited','focused','thinking','analyzing','suspicious',
                 'error','sleepy','success','warning','building','deploying','brainstorm','broken'];
    for (var i = 0; i < names.length; i++) {
      new Image().src = '/expressions/' + names[i] + '.webp';
      // Success never blinks (see blink) and ships no -blink frame.
      if (names[i] === 'success') continue;
      new Image().src = '/expressions/' + names[i] + '-blink.webp';
    }
  }
  window.floydMascot = { mood: setMood, place: place, blink: blink, current: function () { return MOOD; } };
  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', start);
  else start();
})();
