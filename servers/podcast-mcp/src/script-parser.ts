import { ScriptLine, ParsedScript } from "./types";

const SPEAKER_LINE_REGEX = /^([A-Za-z][A-Za-z0-9_ ]{0,30}):\s*(.+)$/;
const DIRECTION_REGEX = /\(([^)]+)\)/g;
const AMBIENT_REGEX = /\[ambient:\s*([^\]]+)\]/gi;

export function parseScript(scriptText: string): ParsedScript {
  const rawLines = scriptText.split("\n").map((l) => l.trim()).filter((l) => l.length > 0);
  const lines: ScriptLine[] = [];
  const speakerSet = new Set<string>();
  const ambientCues: string[] = [];
  let lineNumber = 0;

  for (const raw of rawLines) {
    // Check for ambient cues on standalone lines
    const ambientOnlyMatch = raw.match(/^\[ambient:\s*([^\]]+)\]$/i);
    if (ambientOnlyMatch) {
      ambientCues.push(ambientOnlyMatch[1].trim());
      continue;
    }

    const speakerMatch = raw.match(SPEAKER_LINE_REGEX);
    if (!speakerMatch) {
      continue; // Skip non-dialogue lines (comments, blank, etc.)
    }

    lineNumber++;
    const speaker = speakerMatch[1].trim();
    let dialogue = speakerMatch[2].trim();
    speakerSet.add(speaker);

    // Extract stage directions
    const directions: string[] = [];
    let dirMatch: RegExpExecArray | null;
    const dirRegex = new RegExp(DIRECTION_REGEX.source, DIRECTION_REGEX.flags);
    while ((dirMatch = dirRegex.exec(dialogue)) !== null) {
      directions.push(dirMatch[1].trim());
    }

    // Extract ambient cues from within dialogue
    const ambientRegex = new RegExp(AMBIENT_REGEX.source, AMBIENT_REGEX.flags);
    let ambMatch: RegExpExecArray | null;
    while ((ambMatch = ambientRegex.exec(dialogue)) !== null) {
      ambientCues.push(ambMatch[1].trim());
    }

    // Remove stage directions and ambient cues from spoken text
    dialogue = dialogue
      .replace(DIRECTION_REGEX, "")
      .replace(AMBIENT_REGEX, "")
      .trim();

    // Skip lines that are purely stage directions with no dialogue
    if (dialogue.length === 0) {
      continue;
    }

    const line: ScriptLine = {
      speaker,
      dialogue,
      lineNumber,
    };

    if (directions.length > 0) {
      line.direction = directions.join(", ");
    }

    if (ambientCues.length > 0) {
      line.ambient = ambientCues[ambientCues.length - 1];
    }

    lines.push(line);
  }

  return {
    lines,
    speakers: Array.from(speakerSet),
    totalLines: lines.length,
    ambientCues: [...new Set(ambientCues)],
  };
}
