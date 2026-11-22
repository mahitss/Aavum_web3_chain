'use client';

import { useEffect, useRef } from 'react';
import * as THREE from 'three';

export function Topology({ pulse }: { pulse: number }) {
  const hostRef = useRef<HTMLDivElement>(null);
  const pulseRef = useRef(pulse);
  pulseRef.current = pulse;

  useEffect(() => {
    const host = hostRef.current;
    if (!host) return;

    const scene = new THREE.Scene();
    const camera = new THREE.PerspectiveCamera(42, 1, 0.1, 50);
    camera.position.set(0, 0.85, 7.4);
    camera.lookAt(0, 0.05, 0);

    const renderer = new THREE.WebGLRenderer({ antialias: true, alpha: true });
    renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2));
    host.appendChild(renderer.domElement);

    const positions = [
      new THREE.Vector3(-2.55, 0.72, 0),
      new THREE.Vector3(0, -1.2, 0),
      new THREE.Vector3(2.55, 0.72, 0),
    ];
    const colors = [0x7aa2ff, 0x3ee0c5, 0xe7b549];
    const meshes = positions.map((position, index) => {
      const mesh = new THREE.Mesh(
        new THREE.IcosahedronGeometry(index === 1 ? 0.46 : 0.34, 1),
        new THREE.MeshStandardMaterial({
          color: colors[index],
          emissive: colors[index],
          emissiveIntensity: 0.55,
          roughness: 0.32,
          metalness: 0.18,
        }),
      );
      mesh.position.copy(position);
      scene.add(mesh);
      return mesh;
    });

    const ring = new THREE.Mesh(
      new THREE.TorusGeometry(0.72, 0.012, 12, 48),
      new THREE.MeshBasicMaterial({ color: 0x3ee0c5, transparent: true, opacity: 0.7 }),
    );
    ring.position.copy(positions[1]);
    ring.rotation.x = Math.PI / 2.4;
    scene.add(ring);

    const edges: Array<[number, number]> = [
      [0, 1],
      [1, 2],
      [2, 0],
    ];
    edges.forEach(([from, to]) => {
      const geometry = new THREE.BufferGeometry().setFromPoints([positions[from], positions[to]]);
      scene.add(
        new THREE.Line(
          geometry,
          new THREE.LineBasicMaterial({ color: 0x314556, transparent: true, opacity: 0.95 }),
        ),
      );
    });

    scene.add(new THREE.AmbientLight(0xffffff, 0.4));
    const light = new THREE.PointLight(0x3ee0c5, 12, 16);
    light.position.set(0, 2.2, 3.5);
    scene.add(light);

    type Particle = { mesh: THREE.Mesh; active: boolean; edge: number; t: number };
    const particles: Particle[] = Array.from({ length: 42 }, () => {
      const mesh = new THREE.Mesh(
        new THREE.SphereGeometry(0.042, 8, 8),
        new THREE.MeshBasicMaterial({ color: 0xf7fffb }),
      );
      mesh.visible = false;
      scene.add(mesh);
      return { mesh, active: false, edge: 0, t: 0 };
    });

    const spawn = (edge: number) => {
      const slot = particles.find((particle) => !particle.active);
      if (!slot) return;
      slot.active = true;
      slot.edge = edge;
      slot.t = 0;
      slot.mesh.visible = true;
    };

    let lastPulse = pulseRef.current;
    let frame = 0;
    const clock = new THREE.Clock();
    let raf = 0;

    const resize = () => {
      const width = host.clientWidth || 640;
      const height = host.clientHeight || 420;
      camera.aspect = width / height;
      camera.updateProjectionMatrix();
      renderer.setSize(width, height);
    };
    resize();
    const observer = new ResizeObserver(resize);
    observer.observe(host);

    const tick = () => {
      const delta = clock.getDelta();
      frame += 1;
      meshes.forEach((mesh, index) => {
        mesh.rotation.y += delta * (0.28 + index * 0.07);
        mesh.rotation.x += delta * 0.07;
      });
      ring.rotation.z += delta * 0.35;
      if (pulseRef.current !== lastPulse) {
        lastPulse = pulseRef.current;
        edges.forEach((_, index) => {
          spawn(index);
          spawn(index);
          spawn(index);
        });
      } else if (frame % 80 === 0) {
        spawn(frame % edges.length);
      }
      particles.forEach((particle) => {
        if (!particle.active) return;
        particle.t += delta * 0.48;
        const [from, to] = edges[particle.edge];
        particle.mesh.position.lerpVectors(positions[from], positions[to], Math.min(particle.t, 1));
        if (particle.t >= 1) {
          particle.active = false;
          particle.mesh.visible = false;
        }
      });
      renderer.render(scene, camera);
      raf = requestAnimationFrame(tick);
    };
    raf = requestAnimationFrame(tick);

    return () => {
      cancelAnimationFrame(raf);
      observer.disconnect();
      renderer.dispose();
      if (renderer.domElement.parentNode === host) {
        host.removeChild(renderer.domElement);
      }
    };
  }, []);

  return <div ref={hostRef} className="absolute inset-0" />;
}
