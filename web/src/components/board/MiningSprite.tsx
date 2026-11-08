import type { ModelFamily } from "../../lib/workModel";

/**
 * A tiny pixel-art miner swinging a T-shaped pickaxe, shown in a card's status
 * pill while a model builds the ticket: a crab for Claude, a terminal for
 * Codex. Drawn on a 24×14 grid; motion lives in components.css and stops for
 * people who prefer reduced motion.
 */
export function MiningSprite({ family }: { family: ModelFamily }) {
  return (
    <svg className={`miner miner--${family}`} viewBox="0 0 24 14" width="24" height="14" aria-hidden="true" data-family={family}>
      <rect className="miner__ground" x="0" y="11" width="24" height="1" />
      <rect className="miner__rock" x="19" y="10" width="3" height="1" />
      <g className="miner__body">{family === "claude" ? <Crab /> : <Terminal />}</g>
      <g className="miner__pick">
        <rect className="miner__handle" x="16" y="2" width="1" height="5" />
        <rect className="miner__head" x="14" y="1" width="5" height="1" />
      </g>
      <rect className="miner__dirt" x="21" y="9" width="1" height="1" />
      <rect className="miner__dirt miner__dirt--2" x="20" y="9" width="1" height="1" />
    </svg>
  );
}

function Crab() {
  return (
    <>
      <rect className="miner__shell" x="4" y="4" width="10" height="5" />
      <rect className="miner__eye" x="6" y="5" width="1" height="2" />
      <rect className="miner__eye" x="11" y="5" width="1" height="2" />
      <rect className="miner__shell" x="1" y="5" width="1" height="2" />
      <rect className="miner__shell" x="2" y="6" width="2" height="1" />
      <rect className="miner__shell" x="14" y="6" width="2" height="1" />
      {[5, 7, 10, 12].map(x => <rect key={x} className="miner__shell" x={x} y="9" width="1" height="2" />)}
    </>
  );
}

function Terminal() {
  return (
    <>
      <rect className="miner__shell" x="3" y="3" width="11" height="7" rx="1.2" />
      <rect className="miner__glyph" x="5" y="5" width="1" height="1" />
      <rect className="miner__glyph" x="6" y="6" width="1" height="1" />
      <rect className="miner__glyph" x="5" y="7" width="1" height="1" />
      <rect className="miner__glyph miner__cursor" x="8" y="7" width="3" height="1" />
      <rect className="miner__shell" x="14" y="6" width="2" height="1" />
      <rect className="miner__shell" x="5" y="10" width="1" height="1" />
      <rect className="miner__shell" x="11" y="10" width="1" height="1" />
    </>
  );
}
