import QtQuick

Item {
    id: ring

    property real orbitRadius: 220
    property color accent: "#ffffff"
    property real focusAngle: -90

    onOrbitRadiusChanged: {
        fixedRings.requestPaint();
        focusSweep.requestPaint();
    }
    onAccentChanged: focusSweep.requestPaint()
    onFocusAngleChanged: focusSweep.requestPaint()

    Canvas {
        id: fixedRings

        anchors.fill: parent
        antialiasing: true
        onPaint: {
            const ctx = getContext("2d");
            ctx.reset();
            ctx.translate(width / 2, height / 2);
            const inner = Math.max(48, ring.orbitRadius - 118);
            const tile = ring.orbitRadius + 58;
            const outer = ring.orbitRadius + 122;
            ctx.lineWidth = 1;
            ctx.strokeStyle = Qt.rgba(1, 1, 1, 0.18);
            ctx.setLineDash([]);
            ctx.beginPath();
            ctx.arc(0, 0, inner, 0, Math.PI * 2);
            ctx.stroke();
            ctx.strokeStyle = Qt.rgba(1, 1, 1, 0.28);
            ctx.setLineDash([2, 8]);
            ctx.beginPath();
            ctx.arc(0, 0, tile, 0, Math.PI * 2);
            ctx.stroke();
            ctx.strokeStyle = Qt.rgba(1, 1, 1, 0.12);
            ctx.setLineDash([14, 12]);
            ctx.beginPath();
            ctx.arc(0, 0, outer, 0, Math.PI * 2);
            ctx.stroke();
        }
    }

    Item {
        anchors.fill: parent

        Canvas {
            anchors.fill: parent
            antialiasing: true
            onPaint: {
                const ctx = getContext("2d");
                ctx.reset();
                ctx.translate(width / 2, height / 2);
                ctx.strokeStyle = Qt.rgba(1, 1, 1, 0.34);
                ctx.lineWidth = 1;
                ctx.setLineDash([]);
                const radius = ring.orbitRadius + 92;
                for (let i = 0; i < 96; i++) {
                    const a = (i / 96) * Math.PI * 2;
                    const longTick = i % 8 === 0;
                    const start = radius - (longTick ? 13 : 5);
                    ctx.beginPath();
                    ctx.moveTo(Math.cos(a) * start, Math.sin(a) * start);
                    ctx.lineTo(Math.cos(a) * radius, Math.sin(a) * radius);
                    ctx.stroke();
                }
            }
        }

        RotationAnimator on rotation {
            from: 0
            to: 360
            duration: 110000
            loops: Animation.Infinite
            running: true
        }

    }

    Canvas {
        id: focusSweep

        anchors.fill: parent
        antialiasing: true
        rotation: ring.focusAngle + 90
        onPaint: {
            const ctx = getContext("2d");
            ctx.reset();
            ctx.translate(width / 2, height / 2);
            ctx.strokeStyle = ring.accent;
            ctx.lineWidth = 2;
            ctx.lineCap = "square";
            ctx.setLineDash([]);
            const outer = ring.orbitRadius + 92;
            const inner = Math.max(48, ring.orbitRadius - 102);
            ctx.beginPath();
            ctx.arc(0, 0, outer, -Math.PI / 2 - 0.16, -Math.PI / 2 + 0.16);
            ctx.stroke();
            ctx.beginPath();
            ctx.arc(0, 0, inner, -Math.PI / 2 - 0.1, -Math.PI / 2 + 0.1);
            ctx.stroke();
            ctx.beginPath();
            ctx.moveTo(0, -outer);
            ctx.lineTo(0, -(outer + 16));
            ctx.stroke();
        }

        Behavior on rotation {
            NumberAnimation {
                duration: 180
                easing.type: Easing.OutCubic
            }

        }

    }

}
