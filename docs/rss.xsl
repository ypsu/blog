<?xml version="1.0" encoding="utf-8"?>
<xsl:stylesheet version="3.0" xmlns:xsl="http://www.w3.org/1999/XSL/Transform">
  <xsl:output method="html" encoding="utf-8"/>
  <xsl:template match="/rss/channel">
    <title>rss</title>
    <meta charset='utf-8' />
    <meta name='viewport' content='width=device-width,initial-scale=1' />
    <p>most recent posts on notech.ie:</p>
    <ul>
    <xsl:for-each select="/rss/channel/item">
      <li>
        <a><xsl:attribute name="href"><xsl:value-of select="link"/></xsl:attribute>
        <xsl:value-of select="title"/></a>:
        <xsl:value-of select="description"/>
      </li>
    </xsl:for-each>
    </ul>
  </xsl:template>
</xsl:stylesheet>