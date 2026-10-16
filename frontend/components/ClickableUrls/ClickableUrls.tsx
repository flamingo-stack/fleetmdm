import React from "react";
import DOMPurify from "dompurify";
import classnames from "classnames";

interface IClickableUrls {
  text: string;
  className?: string;
}

const baseClass = "clickable-urls";

const urlReplacer = (match: string) => {
  // Strip trailing punctuation that is unlikely to be part of the intended
  // URL (e.g. a period ending a sentence, or a trailing comma/paren) so the
  // href and displayed text refer to the same, correctly-bounded URL.
  const trailingPunctuationMatch = match.match(/[).,;:!?]+$/);
  const trailingPunctuation = trailingPunctuationMatch
    ? trailingPunctuationMatch[0]
    : "";
  const trimmedMatch = trailingPunctuation
    ? match.slice(0, match.length - trailingPunctuation.length)
    : match;

  const url = trimmedMatch.startsWith("http")
    ? trimmedMatch
    : `https://${trimmedMatch}`;

  // Validate that we end up with a well-formed http(s) URL before rendering
  // an anchor tag. If validation fails, render the original matched text
  // unmodified (no link) to avoid producing an unexpected href target.
  try {
    const parsed = new URL(url);
    if (parsed.protocol !== "http:" && parsed.protocol !== "https:") {
      return match;
    }
  } catch (e) {
    return match;
  }

  return `<a href="${url}" target="_blank" rel="noreferrer">
      ${trimmedMatch}
    </a>${trailingPunctuation}`;
};

const ClickableUrls = ({ text, className }: IClickableUrls): JSX.Element => {
  const clickableUrlClasses = classnames(baseClass, className);

  // Regex to find case insensitive URLs and replace with link
  const textWithLinks = text.replaceAll(
    /(((https?)?(:\/\/))|((https?)?(:\/\/)?(www\.)))[-a-zA-Z0-9@:%._+~#=]{1,256}\.[a-zA-Z0-9()]{1,6}\b([-a-zA-Z0-9()@:%_+.~#?&//=]*)/g,
    urlReplacer
  );
  const sanitizedTextWithLinks = DOMPurify.sanitize(textWithLinks, {
    ADD_ATTR: ["target"], // Allows opening in a new tab
  });

  return (
    <div
      className={clickableUrlClasses}
      dangerouslySetInnerHTML={{ __html: sanitizedTextWithLinks }}
    />
  );
};
export default ClickableUrls;
