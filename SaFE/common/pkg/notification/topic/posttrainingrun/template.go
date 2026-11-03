/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package posttrainingrun

const emailTemplate = `<!DOCTYPE html>
<html>
  <head>
    <meta charset="UTF-8" />
    <title>Post Training Notification</title>
  </head>
  <body style="font-family: Arial, sans-serif; background-color: #f7f9fc; margin: 0; padding: 0;">
    <table width="100%" cellpadding="0" cellspacing="0" style="max-width: 600px; margin: 30px auto; background-color: #ffffff; border-radius: 10px; box-shadow: 0 2px 6px rgba(0,0,0,0.05);">
      <tr>
        <td style="background-color: #2b6cb0; color: #ffffff; padding: 16px 24px; border-top-left-radius: 10px; border-top-right-radius: 10px;">
          <h2 style="margin: 0;">Post Training Notification</h2>
        </td>
      </tr>
      <tr>
        <td style="padding: 24px;">
          <p>Hello,</p>
          <p>Your Post Training run <b>{{.Heading}}</b>:</p>

          <table cellpadding="6" cellspacing="0" width="100%" style="border-collapse: collapse; margin-top: 10px;">
            <tr>
              <td width="140" style="color: #555; font-weight: bold;">Run</td>
              <td>{{.Run.DisplayName}} ({{.Run.RunID}})</td>
            </tr>
            <tr>
              <td style="color: #555; font-weight: bold;">Base model</td>
              <td>{{.Run.BaseModel}}</td>
            </tr>
            <tr>
              <td style="color: #555; font-weight: bold;">Train type</td>
              <td>{{.Run.TrainType}}</td>
            </tr>
            <tr>
              <td style="color: #555; font-weight: bold;">Status</td>
              <td style="color: {{.StatusColor}}; font-weight: bold;">{{.Run.Status}}{{if .Run.FailedStage}} at {{.Run.FailedStage}}{{end}}</td>
            </tr>
            {{if .Run.Duration}}
            <tr>
              <td style="color: #555; font-weight: bold;">Duration</td>
              <td>{{.Run.Duration}}</td>
            </tr>
            {{end}}
            {{if .Run.MainMetric}}
            <tr>
              <td style="color: #555; font-weight: bold;">Main metric</td>
              <td>{{.Run.MainMetric}}</td>
            </tr>
            {{end}}
            {{range $name, $score := .Run.Scores}}
            <tr>
              <td style="color: #555; font-weight: bold;">{{$name}}</td>
              <td>{{printf "%.4g" $score}}</td>
            </tr>
            {{end}}
            {{if .Run.Message}}
            <tr>
              <td style="color: #555; font-weight: bold;">Details</td>
              <td>{{.Run.Message}}</td>
            </tr>
            {{end}}
          </table>

          <p style="margin-top: 20px;">
            You can view more details on the console:<br />
            <a href="{{.URL}}" style="color: #2b6cb0; text-decoration: none;">{{.URL}}</a>
          </p>

          <p style="margin-top: 30px; color: #666;">
            --<br />
            Primus SaFE<br />
            <br />(Stability and Fault Endurance)<br />
            (This is an automated message, please do not reply.)
          </p>
        </td>
      </tr>
    </table>
  </body>
</html>
`
